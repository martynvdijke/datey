import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:webview_flutter/webview_flutter.dart';

import '../utils/cookies.dart';

/// Hosts the server's OIDC authorization flow in an in-app WebView.
///
/// Pops with the captured `session` cookie value on success, or `null` if the
/// user backs out. The cookie is `HttpOnly`, so it cannot be read from
/// JavaScript; it is read from the native cookie store over a small platform
/// channel (`xyz.vandijke.datey/cookies`).
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
  bool _captured = false;
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    _controller = WebViewController()
      ..setJavaScriptMode(JavaScriptMode.unrestricted)
      ..setNavigationDelegate(
        NavigationDelegate(
          onPageStarted: (_) {
            if (mounted) setState(() => _loading = true);
          },
          onPageFinished: (_) {
            if (mounted) setState(() => _loading = false);
            _maybeCaptureSession();
          },
        ),
      )
      ..loadRequest(Uri.parse('${widget.baseUrl}/api/auth/oidc/login'));
  }

  Future<void> _maybeCaptureSession() async {
    if (_captured) return;
    try {
      final raw = await OidcWebViewScreen.cookieChannel
          .invokeMethod<String>('getCookies', {'url': widget.baseUrl});
      final session = parseCookieHeader(raw, 'session');
      if (session != null && session.isNotEmpty) {
        _captured = true;
        if (mounted) Navigator.of(context).pop(session);
      }
    } on MissingPluginException {
      // Platform channel unavailable (e.g. non-Android host) — leave the user
      // on the WebView to finish manually.
    } on PlatformException {
      // Ignore transient failures and retry on the next page load.
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Sign in with OIDC'),
        leading: IconButton(
          icon: const Icon(Icons.close),
          tooltip: 'Cancel',
          onPressed: () => Navigator.of(context).pop(),
        ),
      ),
      body: Stack(
        children: [
          WebViewWidget(controller: _controller),
          if (_loading) const LinearProgressIndicator(),
        ],
      ),
    );
  }
}
