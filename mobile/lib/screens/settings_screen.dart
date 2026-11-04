import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:url_launcher/url_launcher.dart';
import '../providers/auth_provider.dart';
import '../services/api_client.dart';
import '../services/storage_service.dart';

class SettingsScreen extends ConsumerWidget {
  const SettingsScreen({super.key});

  Future<void> _openLegal(BuildContext context, String path) async {
    final base = await StorageService().getBaseUrl();
    if (base == null || base.isEmpty || !context.mounted) return;
    final normalized = base.endsWith('/') ? base.substring(0, base.length - 1) : base;
    final uri = Uri.parse('$normalized$path');
    final ok = await launchUrl(uri, mode: LaunchMode.externalApplication);
    if (!ok && context.mounted) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Could not open the page in a browser')),
      );
    }
  }

  Future<void> _deleteAccount(BuildContext context, WidgetRef ref, String username) async {
    final base = await StorageService().getBaseUrl();
    if (base == null || base.isEmpty || !context.mounted) return;
    final confirmed = await showDialog<bool>(
      context: context,
      barrierDismissible: false,
      builder: (_) => _DeleteAccountDialog(
        api: ApiClient(storage: StorageService(), baseUrl: base),
        username: username,
      ),
    );
    if (confirmed != true || !context.mounted) return;
    await ref.read(authStateProvider.notifier).logout();
    if (!context.mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(
      const SnackBar(content: Text('Your account has been deleted')),
    );
  }

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
          Card(
            child: Column(
              children: [
                ListTile(
                  leading: Icon(Icons.privacy_tip_outlined, color: scheme.primary),
                  title: const Text('Privacy policy'),
                  subtitle: const Text('What is stored, why, and who processes it'),
                  trailing: const Icon(Icons.open_in_new),
                  onTap: () => _openLegal(context, '/privacy'),
                ),
                const Divider(height: 1),
                ListTile(
                  leading: Icon(Icons.description_outlined, color: scheme.primary),
                  title: const Text('Terms of use'),
                  subtitle: const Text('Rules for using this instance'),
                  trailing: const Icon(Icons.open_in_new),
                  onTap: () => _openLegal(context, '/terms'),
                ),
              ],
            ),
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
          const SizedBox(height: 16),
          auth.maybeWhen(
            data: (u) => u == null
                ? const SizedBox.shrink()
                : OutlinedButton.icon(
                    onPressed: () => _deleteAccount(context, ref, u.username),
                    icon: const Icon(Icons.delete_forever_outlined),
                    label: const Text('Delete account'),
                    style: OutlinedButton.styleFrom(
                      foregroundColor: scheme.error,
                      side: BorderSide(color: scheme.error.withValues(alpha: 0.6)),
                    ),
                  ),
            orElse: () => const SizedBox.shrink(),
          ),
        ],
      ),
    );
  }
}

/// Two-step account deletion: an explanation list and an exact-username (or
/// DELETE) check before the destructive call. On failure the dialog stays open
/// and keeps whatever the member typed.
class _DeleteAccountDialog extends StatefulWidget {
  const _DeleteAccountDialog({required this.api, required this.username});

  final ApiClient api;
  final String username;

  @override
  State<_DeleteAccountDialog> createState() => _DeleteAccountDialogState();
}

class _DeleteAccountDialogState extends State<_DeleteAccountDialog> {
  static const _effects = [
    'Your username, email, password hash and preferences are erased.',
    'Every session and API token is revoked, including this phone.',
    'Push subscriptions and notification channels are removed.',
    'The shared address book stays for the other people on this server.',
    'This cannot be undone.',
  ];

  final _confirm = TextEditingController();
  bool _busy = false;
  String? _error;

  @override
  void dispose() {
    _confirm.dispose();
    super.dispose();
  }

  bool get _matches {
    final text = _confirm.text.trim();
    return text == widget.username || text == 'DELETE';
  }

  Future<void> _submit() async {
    if (_busy || !_matches) return;
    setState(() {
      _busy = true;
      _error = null;
    });
    try {
      await widget.api.deleteAccount(widget.username);
      if (!mounted) return;
      Navigator.of(context).pop(true);
    } catch (error) {
      if (!mounted) return;
      setState(() {
        _busy = false;
        _error = _message(error);
      });
    }
  }

  String _message(Object error) {
    if (error is ApiException) return error.message;
    return 'Could not delete the account. Try again.';
  }

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final scheme = theme.colorScheme;
    final hint = theme.textTheme.bodySmall?.copyWith(color: scheme.onSurfaceVariant);
    return AlertDialog(
      title: const Text('Delete your account?'),
      content: SingleChildScrollView(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text('Deleting the account changes these things:', style: hint),
            const SizedBox(height: 12),
            for (final effect in _effects)
              Padding(
                padding: const EdgeInsets.only(bottom: 8),
                child: Row(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Icon(Icons.circle, size: 6, color: scheme.error),
                    const SizedBox(width: 8),
                    Expanded(child: Text(effect, style: theme.textTheme.bodyMedium)),
                  ],
                ),
              ),
            const SizedBox(height: 8),
            TextField(
              controller: _confirm,
              autofocus: true,
              enabled: !_busy,
              decoration: InputDecoration(
                labelText: 'Type ${widget.username} or DELETE to confirm',
                border: const OutlineInputBorder(),
              ),
              onChanged: (_) => setState(() {}),
              onSubmitted: (_) => _submit(),
            ),
            if (_error != null) ...[
              const SizedBox(height: 12),
              Text(_error!, style: TextStyle(color: scheme.error)),
            ],
          ],
        ),
      ),
      actions: [
        TextButton(
          onPressed: _busy ? null : () => Navigator.of(context).pop(false),
          child: const Text('Cancel'),
        ),
        FilledButton(
          onPressed: _busy || !_matches ? null : _submit,
          style: FilledButton.styleFrom(backgroundColor: scheme.error, foregroundColor: scheme.onError),
          child: _busy
              ? const SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2))
              : const Text('Delete account'),
        ),
      ],
    );
  }
}
