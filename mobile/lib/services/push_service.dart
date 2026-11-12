import 'dart:async';
import 'dart:io' show Platform;

import 'package:firebase_core/firebase_core.dart';
import 'package:firebase_messaging/firebase_messaging.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

import 'api_client.dart';
import 'storage_service.dart';

/// Android notification channel used by the server's FCM payloads
/// (`android.notification.channel_id`). Created natively in MainActivity.kt so
/// it exists before the first notification arrives.
const String pushChannelId = 'datey_new_items';

/// Background/terminated handler. The payload contains a `notification` block,
/// so Android displays it without any work here; this handler only exists so
/// the SDK has a registered entry point (and for future data-only messages).
@pragma('vm:entry-point')
Future<void> dateyFirebaseMessagingBackgroundHandler(
  RemoteMessage message,
) async {
  try {
    if (Firebase.apps.isEmpty) {
      await Firebase.initializeApp();
    }
  } catch (e) {
    debugPrint('Push: background handler could not initialise Firebase: $e');
  }
}

/// Android FCM device registration and notification routing.
///
/// Everything is best-effort: a missing Firebase config, a denied permission,
/// an offline server or a rejected token must never break the app.
///
/// Lifecycle:
///  * [start] once after the first frame (called from `app.dart`).
///  * [onLogin] after a session is established, [onLogout] before the session
///    is cleared (called from `auth_provider.dart`).
class PushService {
  PushService({StorageService? storage})
    : _storage = storage ?? StorageService();

  /// App-wide instance, mirroring the `Telemetry.instance` pattern already
  /// used by this project.
  static final PushService instance = PushService();

  final StorageService _storage;

  GlobalKey<NavigatorState>? _navigatorKey;
  GlobalKey<ScaffoldMessengerState>? _scaffoldMessengerKey;
  GoRouter? _router;

  bool _started = false;
  bool _available = false;
  bool _signedIn = false;

  String? _token;
  String? _pendingUrl;

  StreamSubscription<String>? _tokenRefreshSub;
  StreamSubscription<RemoteMessage>? _foregroundSub;
  StreamSubscription<RemoteMessage>? _openedSub;

  /// True when Firebase initialised, i.e. the app was built with a valid
  /// `android/app/google-services.json` and the platform supports FCM.
  bool get isAvailable => _available;

  /// Wires the app-level navigation handles. Safe to call on every build.
  void configure({
    GlobalKey<NavigatorState>? navigatorKey,
    GlobalKey<ScaffoldMessengerState>? scaffoldMessengerKey,
    GoRouter? router,
  }) {
    _navigatorKey = navigatorKey ?? _navigatorKey;
    _scaffoldMessengerKey = scaffoldMessengerKey ?? _scaffoldMessengerKey;
    _router = router ?? _router;
  }

  /// Initialises Firebase (only if the platform config exists), asks for the
  /// notification permission, registers the current token and subscribes to
  /// the messaging streams. Idempotent; never throws.
  Future<void> start() async {
    if (_started) return;
    _started = true;

    // FCM is Android-only in this app. Desktop (tests) and iOS are skipped.
    if (kIsWeb || !Platform.isAndroid) return;

    try {
      if (Firebase.apps.isEmpty) {
        // On Android without google-services.json this throws
        // `[core/not-initialized]` / `google-services.json missing`; the catch
        // below degrades to "push disabled" instead of crashing.
        await Firebase.initializeApp();
      }
    } catch (e) {
      debugPrint('Push: Firebase not configured, push disabled ($e)');
      return;
    }

    _available = true;
    try {
      FirebaseMessaging.onBackgroundMessage(
        dateyFirebaseMessagingBackgroundHandler,
      );

      final messaging = FirebaseMessaging.instance;
      await messaging.requestPermission(alert: true, badge: true, sound: true);

      // The server sends to the FCM registration token (`message.token`), so
      // keep using getToken/onTokenRefresh even though this SDK version
      // deprecates them in favour of register()/onRegistered, which expose the
      // Firebase Installation ID instead.
      // ignore: deprecated_member_use
      _tokenRefreshSub = messaging.onTokenRefresh.listen((token) {
        _token = token;
        if (_signedIn) unawaited(_register(token));
      });
      _foregroundSub = FirebaseMessaging.onMessage.listen(_onForegroundMessage);
      _openedSub = FirebaseMessaging.onMessageOpenedApp.listen(
        _onMessageOpened,
      );

      // ignore: deprecated_member_use
      _token = await messaging.getToken();
      if (_signedIn && _token != null) {
        await _register(_token!);
      }

      // Terminated app: the tap that launched us.
      final initial = await messaging.getInitialMessage();
      if (initial != null) _onMessageOpened(initial);
    } catch (e) {
      debugPrint('Push: messaging setup failed ($e)');
    }
  }

  /// Called after a successful login/session validation. Registers the device
  /// and flushes a notification tap that arrived before sign-in.
  Future<void> onLogin() async {
    _signedIn = true;
    final token = _token;
    if (token != null && token.isNotEmpty) {
      await _register(token);
    }
    _flushPendingUrl();
  }

  /// Called by the auth notifier *before* the session is cleared, so the
  /// DELETE request still carries valid credentials.
  Future<void> onLogout() async {
    _signedIn = false;
    var token = _token;
    _token = null;
    if (token == null || token.isEmpty) {
      token = await _readTokenSafely();
    }
    if (token == null || token.isEmpty) return;
    try {
      final client = await _client();
      // Never let a slow/unreachable server delay sign-out.
      await client?.unregisterDevice(token).timeout(const Duration(seconds: 3));
    } catch (e) {
      debugPrint('Push: could not unregister device ($e)');
    }
  }

  /// Releases the stream subscriptions. Only used by tests.
  @visibleForTesting
  Future<void> dispose() async {
    await _tokenRefreshSub?.cancel();
    await _foregroundSub?.cancel();
    await _openedSub?.cancel();
    _tokenRefreshSub = null;
    _foregroundSub = null;
    _openedSub = null;
    _started = false;
  }

  // ── internals ─────────────────────────────────────────────────────────

  Future<ApiClient?> _client() async {
    final baseUrl = await _storage.getBaseUrl();
    if (baseUrl == null || baseUrl.trim().isEmpty) return null;
    return ApiClient(storage: _storage, baseUrl: baseUrl.trim());
  }

  Future<void> _register(String token) async {
    if (token.isEmpty) return;
    try {
      final client = await _client();
      await client?.registerDevice(token);
    } catch (e) {
      debugPrint('Push: device registration failed ($e)');
    }
  }

  Future<String?> _readTokenSafely() async {
    if (!_available) return null;
    try {
      // ignore: deprecated_member_use
      return await FirebaseMessaging.instance.getToken();
    } catch (_) {
      return null;
    }
  }

  void _onForegroundMessage(RemoteMessage message) {
    final notification = message.notification;
    final title = notification?.title?.trim();
    final body = notification?.body?.trim();
    if ((title == null || title.isEmpty) && (body == null || body.isEmpty)) {
      return;
    }
    final url = _urlFrom(message);
    final context = _navigatorKey?.currentContext;
    final messenger =
        _scaffoldMessengerKey?.currentState ??
        (context == null ? null : ScaffoldMessenger.maybeOf(context));
    if (messenger == null) return;

    messenger.showSnackBar(
      SnackBar(
        content: Text(
          [
            if (title != null && title.isNotEmpty) title,
            if (body != null && body.isNotEmpty) body,
          ].join(' — '),
        ),
        action: url == null
            ? null
            : SnackBarAction(label: 'Open', onPressed: () => _navigate(url)),
      ),
    );
  }

  void _onMessageOpened(RemoteMessage message) {
    final url = _urlFrom(message);
    if (url == null) return;
    _pendingUrl = url;
    _flushPendingUrl();
  }

  String? _urlFrom(RemoteMessage message) {
    final raw = message.data['url'];
    if (raw is! String) return null;
    final url = raw.trim();
    // Only in-app paths: never hand an absolute URL to the router.
    if (url.isEmpty || !url.startsWith('/')) return null;
    return url;
  }

  void _flushPendingUrl() {
    final url = _pendingUrl;
    if (!_signedIn || url == null) return;
    // Signing in rebuilds the GoRouter (router.dart watches the auth state).
    // Give that rebuild a frame plus a short grace period before navigating on
    // the new instance; a cold-start notification tap would otherwise be sent
    // to the router that was built while the session was still loading.
    Future<void>.delayed(const Duration(milliseconds: 150), () {
      if (!_signedIn || _pendingUrl != url) return;
      _pendingUrl = null;
      _navigate(url);
    });
  }

  void _navigate(String url, {int attempt = 0}) {
    WidgetsBinding.instance.addPostFrameCallback((_) {
      final router = _router;
      final mounted = _navigatorKey?.currentState != null;
      if (router == null || !mounted) {
        // The navigator is not on screen yet (start-up or a rebuild in
        // progress); retry briefly, then keep the URL for the next login.
        if (attempt < 10) {
          Future<void>.delayed(const Duration(milliseconds: 50), () {
            _navigate(url, attempt: attempt + 1);
          });
        } else {
          _pendingUrl = url;
        }
        return;
      }
      try {
        router.go(url);
      } catch (e) {
        debugPrint('Push: navigation to $url failed ($e)');
        _pendingUrl = url;
      }
    });
  }
}
