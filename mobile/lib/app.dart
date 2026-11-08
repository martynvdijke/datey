import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'services/storage_service.dart';
import 'src/telemetry/telemetry.dart';
import 'theme.dart';
import 'router.dart';

class DateyApp extends ConsumerWidget {
  const DateyApp({super.key});
  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final router = ref.watch(routerProvider);
    return _TelemetryBootstrap(
      child: MaterialApp.router(
        title: 'Datey',
        theme: AppTheme.light(),
        darkTheme: AppTheme.dark(),
        themeMode: ThemeMode.system,
        routerConfig: router,
        debugShowCheckedModeBanner: false,
      ),
    );
  }
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
