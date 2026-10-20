/// Extracts cookie [name] from a raw `Cookie:` request header value
/// (`a=b; session=xyz`). Used to read the session captured by the OIDC WebView.
String? parseCookieHeader(String? header, String name) {
  if (header == null || header.isEmpty) return null;
  for (final part in header.split(';')) {
    final pair = part.trim();
    final equals = pair.indexOf('=');
    if (equals <= 0) continue;
    if (pair.substring(0, equals).trim() != name) continue;
    final value = pair.substring(equals + 1).trim();
    return value.isEmpty ? null : value;
  }
  return null;
}
