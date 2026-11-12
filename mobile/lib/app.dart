import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'services/push_service.dart';
import 'services/storage_service.dart';
import 'src/telemetry/telemetry.dart';
import 'theme.dart';
import 'router.dart';

/// Global handles used outside the widget tree: push notifications can arrive
/// at any time, including before the first frame has been rendered.
final GlobalKey<NavigatorState> navigatorKey = GlobalKey<NavigatorState>();
final GlobalKey<ScaffoldMessengerState> scaffoldMessengerKey =
    GlobalKey<ScaffoldMessengerState>();

class DateyApp extends ConsumerWidget {
  const DateyApp({super.key});
  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final router = ref.watch(routerProvider);
    // Push taps need the router instance for the current auth state.
    PushService.instance.configure(
      navigatorKey: navigatorKey,
      scaffoldMessengerKey: scaffoldMessengerKey,
      router: router,
    );
    return _PushBootstrap(
      child: _TelemetryBootstrap(
        child: MaterialApp.router(
          title: 'Datey',
          // GoRouter owns the navigator (and receives [navigatorKey] there).
          scaffoldMessengerKey: scaffoldMessengerKey,
          theme: AppTheme.light(),
          darkTheme: AppTheme.dark(),
          themeMode: ThemeMode.system,
          routerConfig: router,
          debugShowCheckedModeBanner: false,
        ),
      ),
    );
  }
}

/// Starts FCM after the first frame. Failures (no google-services.json,
/// permission denied, offline server) are swallowed by [PushService.start], so
/// the app still runs without push support.
class _PushBootstrap extends StatefulWidget {
  const _PushBootstrap({required this.child});

  final Widget child;

  @override
  State<_PushBootstrap> createState() => _PushBootstrapState();
}

class _PushBootstrapState extends State<_PushBootstrap> {
  @override
  void initState() {
    super.initState();
    unawaited(PushService.instance.start());
  }

  @override
  Widget build(BuildContext context) => widget.child;
}

/// Starts telemetry without blocking the first frame. Failures (offline, slow
/// server) are swallowed by [Telemetry.startFromServer], so the app still runs.
class _TelemetryBootstrap extends StatefulWidget {
  const _TelemetryBootstrap({required this.child});

  final Widget child;

  @override
  State<_TelemetryBootstrap> createState() => _TelemetryBootstrapState();
}

class _TelemetryBootstrapState extends State<_TelemetryBootstrap> {
  @override
  void initState() {
    super.initState();
    unawaited(_startTelemetry());
  }

  Future<void> _startTelemetry() async {
    var baseUrl = defaultBaseUrl;
    try {
      final saved = await StorageService().getBaseUrl();
      if (saved != null && saved.trim().isNotEmpty) {
        baseUrl = saved.trim();
      }
    } catch (_) {
      // Secure storage unavailable: keep the hosted default.
    }
    try {
      await Telemetry.instance.startFromServer(baseUrl);
    } catch (_) {
      // Telemetry must never prevent the app from starting.
    }
  }

  @override
  Widget build(BuildContext context) => widget.child;
}
