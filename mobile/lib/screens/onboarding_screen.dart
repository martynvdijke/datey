import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../providers/auth_provider.dart';
import '../services/api_client.dart';
import '../services/storage_service.dart';
import '../utils/validators.dart';
import 'oidc_webview_screen.dart';

class OnboardingScreen extends ConsumerStatefulWidget {
  const OnboardingScreen({super.key});
  @override
  ConsumerState<OnboardingScreen> createState() => _OnboardingScreenState();
}

class _OnboardingScreenState extends ConsumerState<OnboardingScreen> {
  final _formKey = GlobalKey<FormState>();
  final _urlCtrl = TextEditingController();
  final _userCtrl = TextEditingController();
  final _passCtrl = TextEditingController();
  bool _testing = false;
  bool _loggingIn = false;
  String? _urlError;
  String? _loginError;
  bool _obscure = true;
  bool _urlValid = false;
  bool _oidcEnabled = false;
  bool _checkingOidc = false;

  @override
  void initState() {
    super.initState();
    _loadExisting();
  }

  Future<void> _loadExisting() async {
    final storage = StorageService();
    final url = await storage.getBaseUrl();
    if (url != null && mounted) setState(() => _urlCtrl.text = url);
    if (url != null) _checkOidc(url);
  }

  Future<void> _checkOidc(String baseUrl) async {
    setState(() => _checkingOidc = true);
    try {
      final storage = StorageService();
      final client = ApiClient(storage: storage, baseUrl: baseUrl);
      final resp = await client.getOidcStatus();
      if (resp.statusCode == 200 && resp.data is Map) {
        if (mounted) setState(() => _oidcEnabled = resp.data['enabled'] == true);
      }
    } catch (_) {}
    if (mounted) setState(() => _checkingOidc = false);
  }

  Future<void> _testConnection() async {
    final raw = _urlCtrl.text.trim();
    final err = validateServerUrl(raw);
    if (err != null) {
      setState(() => _urlError = err);
      return;
    }
    final normalized = normalizeServerUrl(raw);
    setState(() {
      _testing = true;
      _urlError = null;
    });
    final storage = StorageService();
    final client = ApiClient(storage: storage);
    final ok = await client.testConnection(normalized);
    if (!mounted) return;
    setState(() => _testing = false);
    if (ok) {
      setState(() => _urlValid = true);
      await storage.setBaseUrl(normalized);
      _checkOidc(normalized);
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(content: Text('Connected to $normalized'), behavior: SnackBarBehavior.floating),
        );
      }
    } else {
      setState(() => _urlError = 'Could not reach server. Check the URL.');
    }
  }

  Future<void> _login() async {
    final urlErr = validateServerUrl(_urlCtrl.text);
    if (urlErr != null) {
      setState(() => _urlError = urlErr);
      return;
    }
    if (!_formKey.currentState!.validate()) return;
    final baseUrl = normalizeServerUrl(_urlCtrl.text.trim());
    setState(() {
      _loggingIn = true;
      _loginError = null;
    });
    final ok = await ref.read(authStateProvider.notifier).login(baseUrl, _userCtrl.text.trim(), _passCtrl.text);
    if (!mounted) return;
    setState(() => _loggingIn = false);
    if (!ok) {
      final st = ref.read(authStateProvider);
      st.whenOrNull(error: (e, _) => setState(() => _loginError = e.toString()));
      if (_loginError == null) setState(() => _loginError = 'Login failed. Check your credentials.');
    }
  }

  Future<void> _oidcLogin() async {
    final baseUrl = normalizeServerUrl(_urlCtrl.text.trim());
    final storage = StorageService();
    await storage.setBaseUrl(baseUrl);
    if (!mounted) return;
    final session = await Navigator.of(context).push<String>(
      MaterialPageRoute(
        builder: (_) => OidcWebViewScreen(baseUrl: baseUrl),
      ),
    );
    if (session == null || session.isEmpty) return;
    await storage.persistCookies(['session=$session; Path=/']);
    await ref.read(authStateProvider.notifier).validateSession();
    if (!mounted) return;
    final st = ref.read(authStateProvider);
    if (st.value == null) {
      setState(() => _loginError = 'Sign-in did not complete. Try again.');
    }
  }

  @override
  void dispose() {
    _urlCtrl.dispose();
    _userCtrl.dispose();
    _passCtrl.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return Scaffold(
      body: SafeArea(
        child: SingleChildScrollView(
          padding: const EdgeInsets.fromLTRB(24, 32, 24, 24),
          child: Form(
            key: _formKey,
            child: Column(crossAxisAlignment: CrossAxisAlignment.stretch, children: [
              // Header
              Container(
                width: 64,
                height: 64,
                decoration: BoxDecoration(
                  gradient: LinearGradient(colors: [scheme.primary, scheme.tertiary]),
                  borderRadius: BorderRadius.circular(18),
                ),
                child: const Icon(Icons.cake_rounded, color: Colors.white, size: 32),
              ),
              const SizedBox(height: 20),
              Text('Welcome to Datey',
                  style: TextStyle(fontSize: 28, fontWeight: FontWeight.w900, letterSpacing: -0.8, color: scheme.onSurface)),
              const SizedBox(height: 8),
              Text('Never miss a birthday or anniversary. Sign in to your Datey server to get started.',
                  style: TextStyle(fontSize: 15, color: scheme.onSurfaceVariant, height: 1.4)),
              const SizedBox(height: 32),
              // Server URL card
              Card(
                child: Padding(
                  padding: const EdgeInsets.all(20),
                  child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                    Row(children: [
                      Icon(Icons.dns_rounded, size: 18, color: scheme.primary),
                      const SizedBox(width: 8),
                      Text('Server URL', style: TextStyle(fontWeight: FontWeight.w700, color: scheme.onSurface)),
                    ]),
                    const SizedBox(height: 12),
                    TextFormField(
                      controller: _urlCtrl,
                      decoration: InputDecoration(
                        hintText: 'https://datey.example.com',
                        border: const OutlineInputBorder(),
                        prefixIcon: const Icon(Icons.link),
                        errorText: _urlError,
                        suffixIcon: _urlValid ? Icon(Icons.check_circle, color: scheme.primary) : null,
                      ),
                      keyboardType: TextInputType.url,
                      autocorrect: false,
                      onChanged: (_) => setState(() => _urlValid = false),
                    ),
                    const SizedBox(height: 12),
                    SizedBox(
                      width: double.infinity,
                      child: OutlinedButton.icon(
                        onPressed: _testing ? null : _testConnection,
                        icon: _testing
                            ? const SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2))
                            : const Icon(Icons.wifi_tethering, size: 18),
                        label: Text(_testing ? 'Testing…' : 'Test connection'),
                      ),
                    ),
                    if (_checkingOidc) ...[
                      const SizedBox(height: 8),
                      Row(children: [
                        const SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2)),
                        const SizedBox(width: 8),
                        Text('Checking sign-in options…', style: TextStyle(fontSize: 12, color: scheme.onSurfaceVariant)),
                      ]),
                    ],
                  ]),
                ),
              ),
              const SizedBox(height: 20),
              // Login card
              Card(
                child: Padding(
                  padding: const EdgeInsets.all(20),
                  child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                    Text('Sign in', style: TextStyle(fontWeight: FontWeight.w700, color: scheme.onSurface)),
                    const SizedBox(height: 12),
                    TextFormField(
                      controller: _userCtrl,
                      decoration: const InputDecoration(labelText: 'Username', border: OutlineInputBorder(), prefixIcon: Icon(Icons.person_outline)),
                      validator: (v) => v == null || v.trim().isEmpty ? 'Required' : null,
                      textInputAction: TextInputAction.next,
                    ),
                    const SizedBox(height: 12),
                    TextFormField(
                      controller: _passCtrl,
                      decoration: InputDecoration(
                        labelText: 'Password',
                        border: const OutlineInputBorder(),
                        prefixIcon: const Icon(Icons.lock_outline),
                        suffixIcon: IconButton(
                          icon: Icon(_obscure ? Icons.visibility_off : Icons.visibility),
                          onPressed: () => setState(() => _obscure = !_obscure),
                        ),
                      ),
                      obscureText: _obscure,
                      validator: (v) => v == null || v.isEmpty ? 'Required' : null,
                      onFieldSubmitted: (_) => _login(),
                    ),
                    if (_loginError != null) ...[
                      const SizedBox(height: 12),
                      Container(
                        padding: const EdgeInsets.all(12),
                        decoration: BoxDecoration(color: scheme.errorContainer, borderRadius: BorderRadius.circular(12)),
                        child: Row(children: [
                          Icon(Icons.error_outline, size: 18, color: scheme.onErrorContainer),
                          const SizedBox(width: 8),
                          Expanded(child: Text(_loginError!, style: TextStyle(color: scheme.onErrorContainer, fontSize: 13))),
                        ]),
                      ),
                    ],
                    const SizedBox(height: 16),
                    SizedBox(
                      width: double.infinity,
                      child: FilledButton(
                        onPressed: _loggingIn ? null : _login,
                        child: _loggingIn
                            ? const SizedBox(width: 20, height: 20, child: CircularProgressIndicator(strokeWidth: 2, color: Colors.white))
                            : const Text('Sign in'),
                      ),
                    ),
                    if (_oidcEnabled) ...[
                      const SizedBox(height: 12),
                      Row(children: [
                        const Expanded(child: Divider()),
                        Padding(padding: const EdgeInsets.symmetric(horizontal: 12), child: Text('or', style: TextStyle(color: scheme.onSurfaceVariant))),
                        const Expanded(child: Divider()),
                      ]),
                      const SizedBox(height: 12),
                      SizedBox(
                        width: double.infinity,
                        child: OutlinedButton.icon(
                          onPressed: _oidcLogin,
                          icon: const Icon(Icons.open_in_new, size: 18),
                          label: const Text('Sign in with OIDC'),
                        ),
                      ),
                    ],
                  ]),
                ),
              ),
              const SizedBox(height: 16),
              Text('Your server URL and session are stored securely on this device.',
                  textAlign: TextAlign.center, style: TextStyle(fontSize: 12, color: scheme.onSurfaceVariant)),
            ]),
          ),
        ),
      ),
    );
  }
}
