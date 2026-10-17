String? validateServerUrl(String? value) {
  if (value == null || value.trim().isEmpty) {
    return 'Enter your server URL';
  }
  final v = value.trim();
  final uri = Uri.tryParse(v);
  if (uri == null || !uri.hasScheme || !(uri.scheme == 'http' || uri.scheme == 'https')) {
    return 'Use http:// or https://';
  }
  if (uri.host.isEmpty) return 'Invalid URL';
  return null;
}

String normalizeServerUrl(String raw) {
  var v = raw.trim();
  // Remove trailing slash
  while (v.endsWith('/')) {
    v = v.substring(0, v.length - 1);
  }
  return v;
}
