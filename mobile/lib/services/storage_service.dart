import 'package:flutter_secure_storage/flutter_secure_storage.dart';

class StorageService {
  static const _kBaseUrl = 'datey_base_url';
  static const _kSession = 'datey_session';
  static const _kCsrf = 'datey_csrf_token';

  final FlutterSecureStorage _storage;
  StorageService([FlutterSecureStorage? storage])
      : _storage = storage ?? const FlutterSecureStorage();

  Future<String?> getBaseUrl() => _storage.read(key: _kBaseUrl);
  Future<void> setBaseUrl(String url) => _storage.write(key: _kBaseUrl, value: url);

  Future<String?> getSession() => _storage.read(key: _kSession);
  Future<void> setSession(String? v) async {
    if (v == null || v.isEmpty) {
      await _storage.delete(key: _kSession);
    } else {
      await _storage.write(key: _kSession, value: v);
    }
  }

  /// Store a session captured outside the HTTP client (e.g. the OIDC WebView)
  /// and drop any CSRF token that belonged to the previous session.
  Future<void> saveSession(String session) async {
    await setSession(session);
    await setCsrf(null);
  }

  Future<String?> getCsrf() => _storage.read(key: _kCsrf);
  Future<void> setCsrf(String? v) async {
    if (v == null || v.isEmpty) {
      await _storage.delete(key: _kCsrf);
    } else {
      await _storage.write(key: _kCsrf, value: v);
    }
  }

  Future<void> clearAuth() async {
    await _storage.delete(key: _kSession);
    await _storage.delete(key: _kCsrf);
  }

  Future<void> clearAll() async {
    await _storage.deleteAll();
  }

  /// Parse Set-Cookie headers and persist session/csrf.
  Future<void> persistCookies(List<String> setCookieHeaders) async {
    for (final raw in setCookieHeaders) {
      final lower = raw.toLowerCase();
      if (lower.startsWith('session=')) {
        final v = _extractCookieValue(raw, 'session');
        if (v != null) await setSession(v);
      } else if (lower.startsWith('csrf_token=')) {
        final v = _extractCookieValue(raw, 'csrf_token');
        if (v != null) await setCsrf(v);
      }
    }
  }

  String? _extractCookieValue(String header, String name) {
    // header like "session=abc123; Path=/; HttpOnly"
    final parts = header.split(';');
    for (final p in parts) {
      final trimmed = p.trim();
      if (trimmed.toLowerCase().startsWith('${name.toLowerCase()}=') ) {
        return trimmed.substring(name.length + 1);
      }
    }
    // also handle first part directly
    final first = parts.first.trim();
    final eq = first.indexOf('=');
    if (eq != -1) {
      final k = first.substring(0, eq).trim().toLowerCase();
      if (k == name.toLowerCase()) return first.substring(eq + 1);
    }
    return null;
  }
}
