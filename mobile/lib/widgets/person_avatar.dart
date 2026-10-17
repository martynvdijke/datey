import 'package:cached_network_image/cached_network_image.dart';
import 'package:flutter/material.dart';
import '../services/storage_service.dart';

class PersonAvatar extends StatelessWidget {
  final String? photoUrl;
  final String name;
  final double radius;
  const PersonAvatar({super.key, this.photoUrl, required this.name, this.radius = 24});

  String _initials(String n) {
    final parts = n.trim().split(RegExp(r'\s+'));
    if (parts.isEmpty) return '?';
    if (parts.length == 1) return parts.first.characters.first.toUpperCase();
    return (parts.first.characters.first + parts.last.characters.first).toUpperCase();
  }

  Future<Map<String, String>?> _headers() async {
    if (photoUrl == null || photoUrl!.isEmpty) return null;
    final storage = StorageService();
    final session = await storage.getSession();
    final csrf = await storage.getCsrf();
    if (session == null) return null;
    final cookie = 'session=$session${csrf != null && csrf.isNotEmpty ? '; csrf_token=$csrf' : ''}';
    return {'Cookie': cookie};
  }

  Future<String?> _resolvedUrl() async {
    if (photoUrl == null || photoUrl!.isEmpty) return null;
    if (photoUrl!.startsWith('http')) return photoUrl;
    final storage = StorageService();
    final baseUrl = await storage.getBaseUrl();
    if (baseUrl == null || baseUrl.isEmpty) return null;
    final base = baseUrl.endsWith('/') ? baseUrl.substring(0, baseUrl.length - 1) : baseUrl;
    return '$base$photoUrl';
  }

  @override
  Widget build(BuildContext context) {
    final initials = _initials(name);
    final scheme = Theme.of(context).colorScheme;
    return FutureBuilder<String?>(
      future: _resolvedUrl(),
      builder: (context, snapUrl) {
        final url = snapUrl.data;
        if (url == null || url.isEmpty) return _fallback(context, initials, scheme);
        return FutureBuilder<Map<String, String>?>(
          future: _headers(),
          builder: (context, snapHeaders) {
            return ClipOval(
              child: CachedNetworkImage(
                imageUrl: url,
                httpHeaders: snapHeaders.data,
                width: radius * 2,
                height: radius * 2,
                fit: BoxFit.cover,
                placeholder: (_, _) => _fallback(context, initials, scheme),
                errorWidget: (_, _, _) => _fallback(context, initials, scheme),
              ),
            );
          },
        );
      },
    );
  }

  Widget _fallback(BuildContext context, String initials, ColorScheme scheme) {
    // generate deterministic color from name
    final hash = name.hashCode;
    final hue = (hash % 360).abs().toDouble();
    final bg = HSLColor.fromAHSL(1, hue, 0.6, 0.55).toColor();
    return CircleAvatar(
      radius: radius,
      backgroundColor: bg,
      child: Text(initials,
          style: TextStyle(
              color: Colors.white,
              fontWeight: FontWeight.w800,
              fontSize: radius * 0.7)),
    );
  }
}
