import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:webview_flutter/webview_flutter.dart';

import '../utils/cookies.dart';

/// Errors Android WebView reports for ordinary navigation churn rather than a
/// real failure: redirects abort the previous load (`ERR_ABORTED`), custom
/// schemes (app handoff, `market://`, `intent://`) are unsupported, and the
/// platform may report a cancel type. These must never replace a page that is
/// still loading or already loaded.
bool shouldIgnoreWebResourceError({
  required int errorCode,
  required String description,
  String? errorTypeName,
}) {
  if (errorTypeName == 'cancelled') return true;
  // Android WebViewClient.ERROR_UNSUPPORTED_SCHEME / net::ERR_UNKNOWN_URL_SCHEME.
  if (errorCode == -10) return true;
  if (errorCode == -3) return true; // net::ERR_ABORTED
  final lower = description.toLowerCase();
  return lower.contains('err_aborted') ||
      lower.contains('err_unknown_url_scheme') ||
      lower.contains('operation was canceled') ||
      lower.contains('operation was cancelled');
}

/// Hosts the server's OIDC authorization flow in an in-app WebView.
///
/// Pops with the captured `session` cookie value on success, or `null` if the
/// user backs out. The cookie is `HttpOnly`, so it cannot be read from
/// JavaScript; it is read from the native cookie store.
class OidcWebViewScreen extends StatefulWidget {
  const OidcWebViewScreen({super.key, required this.baseUrl});

  final String baseUrl;

  static const MethodChannel cookieChannel =
      MethodChannel('xyz.vandijke.datey/cookies');

  @override
  State<OidcWebViewScreen> createState() => _OidcWebViewScreenState();
}

class _OidcWebViewScreenState extends State<OidcWebViewScreen> {
  late final WebViewController _controller;
  late final Uri _serverOrigin;

  /// Set once the session has been handed back to the caller, so the WebView
  /// can never pop the route twice.
  bool _completed = false;
  bool _capturing = false;
  bool _captureQueued = false;
  bool _loading = true;
  String? _loadError;
  bool _embeddedBlocked = false;
  // True once a page on the Datey origin has finished/committed. An error that
  // arrives after that (e.g. an aborted redirect) must not replace the page.
  bool _lastServerPageOk = false;

  @override
  void initState() {
    super.initState();
    _serverOrigin = Uri.parse(widget.baseUrl);
    _controller = WebViewController()
      ..setJavaScriptMode(JavaScriptMode.unrestricted)
      ..setNavigationDelegate(
        NavigationDelegate(
          onPageStarted: (_) {
            if (!mounted) return;
            setState(() {
              _loading = true;
              _loadError = null;
              _embeddedBlocked = false;
              _lastServerPageOk = false;
            });
          },
          onPageFinished: (url) {
            if (!mounted) return;
            if (_isServerUrl(url)) {
              _lastServerPageOk = true;
              setState(() {
                _loading = false;
                _loadError = null;
                _embeddedBlocked = false;
              });
              unawaited(_captureSession());
            } else {
              setState(() => _loading = false);
            }
          },
          onUrlChange: (change) {
            if (!mounted) return;
            if (_isServerUrl(change.url)) {
              _lastServerPageOk = true;
              if (_loadError != null || _embeddedBlocked) {
                setState(() {
                  _loadError = null;
                  _embeddedBlocked = false;
                });
              }
              unawaited(_captureSession());
            }
          },
          onHttpError: (error) {
            final status = error.response?.statusCode;
            if (status == 401 || status == 403) {
              unawaited(_handleHttpError(status!, error.request?.uri));
            }
          },
          onWebResourceError: (error) {
            if (!mounted || error.isForMainFrame == false) return;
            if (shouldIgnoreWebResourceError(
              errorCode: error.errorCode,
              description: error.description,
              errorTypeName: error.errorType?.name,
            )) {
              return;
            }
            // A newer page on the server already loaded successfully; a late
            // error for an older navigation must not hide it.
            if (_lastServerPageOk) return;
            final description = error.description;
            final blocked = description.contains('403') ||
                description.toLowerCase().contains('disallow') ||
                description.toLowerCase().contains('blocked');
            setState(() {
              _loading = false;
              _embeddedBlocked = blocked;
              _loadError = description.isEmpty
                  ? 'The sign-in page could not be loaded.'
                  : description;
            });
          },
          onNavigationRequest: (_) => NavigationDecision.navigate,
        ),
      );

    // The identity provider sets its own session cookie on a different origin.
    // OIDC redirects are top-level navigations, so the IdP cookie is normally
    // treated as first-party even though WebView blocks third-party cookies.
    // Clear stale cookies first: an old Datey session would otherwise be
    // captured immediately, before the user has actually authenticated.
    unawaited(_start());
  }

  Future<void> _start() async {
    await _clearCookies();
    if (!mounted || _completed) return;
    _controller.loadRequest(Uri.parse('${widget.baseUrl}/api/auth/oidc/login'));
  }

  Future<void> _clearCookies() async {
    try {
      await OidcWebViewScreen.cookieChannel
          .invokeMethod<void>('clearCookies');
    } catch (_) {
      // Channel unavailable (e.g. non-Android host); use the plugin below.
    }
    try {
      await WebViewCookieManager().clearCookies();
    } catch (_) {
      // Nothing else to try; the request will simply start with old cookies.
    }
  }

  /// Handles a top-level HTTP 401/403. Subresource errors are ignored so a
  /// blocked image or analytics request cannot replace the sign-in page.
  Future<void> _handleHttpError(int status, Uri? requestUri) async {
    if (!mounted) return;
    String? current;
    try {
      current = await _controller.currentUrl();
    } catch (_) {
      return;
    }
    if (!mounted || current == null) return;
    if (requestUri != null &&
        _stripTrailingSlashes(requestUri.toString()) !=
            _stripTrailingSlashes(current)) {
      return;
    }
    setState(() {
      _loading = false;
      _embeddedBlocked = true;
      _loadError = 'The sign-in provider returned HTTP $status.';
    });
  }

  String _stripTrailingSlashes(String url) {
    var value = url;
    while (value.length > 1 && value.endsWith('/')) {
      value = value.substring(0, value.length - 1);
    }
    return value;
  }

  /// True only for URLs on the configured Datey server, never for identity
  /// provider pages. This prevents reading/capturing while the user is still
  /// typing credentials at the IdP.
  bool _isServerUrl(String? url) {
    if (url == null || url.isEmpty) return false;
    final uri = Uri.tryParse(url);
    if (uri == null || !uri.hasScheme || uri.host.isEmpty) return false;
    return uri.scheme == _serverOrigin.scheme &&
        uri.host == _serverOrigin.host &&
        uri.port == _serverOrigin.port;
  }

  /// The WebView may finish navigating before its cookie store is flushed, so
  /// poll for a while instead of giving up after the first read. A trigger that
  /// arrives while a poll is already running queues one more pass, because the
  /// cookie may be committed only after that later navigation change.
  Future<void> _captureSession() async {
    if (_completed) return;
    if (_capturing) {
      _captureQueued = true;
      return;
    }
    _capturing = true;
    try {
      do {
        _captureQueued = false;
        // ~3 seconds total: 12 attempts, 250 ms apart.
        for (var attempt = 0; attempt < 12 && !_completed; attempt++) {
          if (!mounted) return;
          final current = await _controller.currentUrl();
          if (current != null && !_isServerUrl(current)) {
            // We navigated to the identity provider; don't capture or pop here.
            return;
          }
          final session = await _readSessionCookie();
          if (session != null && session.isNotEmpty) {
            if (!mounted) return;
            final url = await _controller.currentUrl();
            if (url != null && !_isServerUrl(url)) continue;
            _completed = true;
            if (mounted) Navigator.of(context).pop(session);
            return;
          }
          if (attempt < 11) {
            await Future.delayed(const Duration(milliseconds: 250));
          }
        }
      } while (_captureQueued && !_completed && mounted);
    } finally {
      _capturing = false;
      _captureQueued = false;
    }
  }

  Future<String?> _readSessionCookie() async {
    try {
      final raw = await OidcWebViewScreen.cookieChannel
          .invokeMethod<String>('getCookies', {'url': widget.baseUrl});
      final session = parseCookieHeader(raw, 'session');
      if (session != null && session.isNotEmpty) return session;
    } on MissingPluginException {
      // Platform channel unavailable (e.g. non-Android host) — fall through to
      // the WebView plugin's cookie store.
    } on PlatformException {
      // Ignore transient failures and retry on the next poll.
    }

    try {
      final cookies =
          await WebViewCookieManager().getCookies(domain: Uri.parse(widget.baseUrl));
      for (final cookie in cookies) {
        if (cookie.name == 'session' && cookie.value.isNotEmpty) {
          return cookie.value;
        }
      }
    } catch (_) {
      // No platform cookie manager available; nothing else to try.
    }
    return null;
  }

  void _reload() {
    setState(() {
      _loadError = null;
      _embeddedBlocked = false;
      _loading = true;
    });
    _controller.loadRequest(Uri.parse('${widget.baseUrl}/api/auth/oidc/login'));
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Sign in with single sign-on'),
        leading: IconButton(
          icon: const Icon(Icons.close),
          tooltip: 'Cancel',
          onPressed: () => Navigator.of(context).pop(),
        ),
      ),
      body: Stack(
        children: [
          if (_loadError == null)
            WebViewWidget(controller: _controller)
          else
            Center(
              child: SingleChildScrollView(
                padding: const EdgeInsets.all(24),
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Icon(Icons.cloud_off_rounded,
                        size: 48, color: Theme.of(context).colorScheme.error),
                    const SizedBox(height: 12),
                    Text(
                      _embeddedBlocked
                          ? 'This sign-in provider blocked the embedded browser.'
                          : 'Could not reach the sign-in page.',
                      style: Theme.of(context).textTheme.titleMedium,
                      textAlign: TextAlign.center,
                    ),
                    if (_embeddedBlocked) ...[
                      const SizedBox(height: 8),
                      Text(
                        'Some identity providers refuse sign-in from an embedded '
                        'WebView. Try "Continue with SSO" again, or sign in with '
                        'your username and password instead.',
                        style: Theme.of(context).textTheme.bodySmall,
                        textAlign: TextAlign.center,
                      ),
                    ],
                    const SizedBox(height: 4),
                    Text(
                      _loadError!,
                      style: Theme.of(context).textTheme.bodySmall,
                      textAlign: TextAlign.center,
                    ),
                    const SizedBox(height: 16),
                    FilledButton.icon(
                      onPressed: _reload,
                      icon: const Icon(Icons.refresh),
                      label: const Text('Try again'),
                    ),
                  ],
                ),
              ),
            ),
          if (_loading) const LinearProgressIndicator(),
        ],
      ),
    );
  }
}
