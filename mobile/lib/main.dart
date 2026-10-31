import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'app.dart';

void main() {
  WidgetsFlutterBinding.ensureInitialized();
  runApp(ProviderScope(
    // Preserve v2 behaviour: surface errors instead of silently retrying forever.
    retry: (retryCount, error) => null,
    child: const DateyApp(),
  ));
}
