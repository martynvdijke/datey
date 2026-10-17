import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../providers/auth_provider.dart';
import '../services/storage_service.dart';

class SettingsScreen extends ConsumerWidget {
  const SettingsScreen({super.key});
  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final auth = ref.watch(authStateProvider);
    final scheme = Theme.of(context).colorScheme;
    return Scaffold(
      appBar: AppBar(title: const Text('Settings')),
      body: ListView(
        padding: const EdgeInsets.fromLTRB(16, 16, 16, 24),
        children: [
          Card(
            child: Padding(
              padding: const EdgeInsets.all(16),
              child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                Row(children: [
                  Icon(Icons.person, color: scheme.primary),
                  const SizedBox(width: 8),
                  Text('Account', style: TextStyle(fontWeight: FontWeight.w800, color: scheme.onSurface)),
                ]),
                const SizedBox(height: 12),
                auth.when(
                  data: (u) => u == null
                      ? Text('Not signed in', style: TextStyle(color: scheme.onSurfaceVariant))
                      : Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                          Text(u.username, style: const TextStyle(fontWeight: FontWeight.w700, fontSize: 16)),
                          Text(u.role, style: TextStyle(color: scheme.onSurfaceVariant, fontSize: 12)),
                          if (u.linkedPersonId != null)
                            Text('Your profile is linked — your own gift ideas stay hidden',
                                style: TextStyle(color: scheme.onSurfaceVariant, fontSize: 12)),
                        ]),
                  loading: () => const CircularProgressIndicator(),
                  error: (e, _) => Text(e.toString(), style: TextStyle(color: scheme.error)),
                ),
              ]),
            ),
          ),
          const SizedBox(height: 16),
          FutureBuilder<String?>(
            future: StorageService().getBaseUrl(),
            builder: (context, snap) {
              if (!snap.hasData) return const SizedBox.shrink();
              return Card(
                child: ListTile(
                  leading: Icon(Icons.dns, color: scheme.primary),
                  title: const Text('Server URL'),
                  subtitle: Text(snap.data ?? ''),
                ),
              );
            },
          ),
          const SizedBox(height: 16),
          FilledButton.icon(
            onPressed: () async {
              final ok = await showDialog<bool>(
                context: context,
                builder: (ctx) => AlertDialog(
                  title: const Text('Sign out?'),
                  content: const Text('You’ll need to sign in again to use the app.'),
                  actions: [
                    TextButton(onPressed: () => Navigator.pop(ctx, false), child: const Text('Cancel')),
                    FilledButton(onPressed: () => Navigator.pop(ctx, true), child: const Text('Sign out')),
                  ],
                ),
              );
              if (ok == true) {
                await ref.read(authStateProvider.notifier).logout();
              }
            },
            icon: const Icon(Icons.logout),
            label: const Text('Sign out'),
            style: FilledButton.styleFrom(backgroundColor: scheme.error, foregroundColor: scheme.onError),
          ),
        ],
      ),
    );
  }
}
