import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import '../providers/people_provider.dart';
import '../services/api_client.dart';
import '../services/storage_service.dart';
import '../widgets/person_avatar.dart';
import '../widgets/empty_state.dart';
import '../utils/formatters.dart';

class PeopleScreen extends ConsumerStatefulWidget {
  const PeopleScreen({super.key});
  @override
  ConsumerState<PeopleScreen> createState() => _PeopleScreenState();
}

class _PeopleScreenState extends ConsumerState<PeopleScreen> {
  final _searchCtrl = TextEditingController();
  bool _syncingCardDav = false;
  bool _syncingImmich = false;
  String? _syncResult;

  @override
  void dispose() {
    _searchCtrl.dispose();
    super.dispose();
  }

  Future<void> _sync(String which) async {
    setState(() {
      if (which == 'carddav') _syncingCardDav = true;
      if (which == 'immich') _syncingImmich = true;
      _syncResult = null;
    });
    try {
      final storage = StorageService();
      final baseUrl = await storage.getBaseUrl();
      if (baseUrl == null) throw Exception('No server URL');
      final client = ApiClient(storage: storage, baseUrl: baseUrl);
      final resp = which == 'carddav' ? await client.syncCardDav() : await client.syncImmich();
      String msg;
      if (resp.statusCode == 200 && resp.data is Map) {
        final m = resp.data as Map;
        if (which == 'carddav') {
          msg = 'CardDAV: pulled ${m['pulled'] ?? 0}, pushed ${m['pushed'] ?? 0}';
        } else {
          msg = 'Immich: scanned ${m['scanned'] ?? 0}, updated ${m['updated'] ?? 0}';
        }
        final errs = m['errors'] as List<dynamic>?;
        if (errs != null && errs.isNotEmpty) msg += ' · ${errs.length} errors';
      } else {
        msg = resp.data is Map ? (resp.data['message'] ?? 'Sync failed') : 'Sync failed';
      }
      if (mounted) setState(() => _syncResult = msg);
      ref.invalidate(peopleProvider);
    } catch (e) {
      if (mounted) setState(() => _syncResult = e.toString());
    } finally {
      if (mounted) {
        setState(() {
          _syncingCardDav = false;
          _syncingImmich = false;
        });
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final async = ref.watch(peopleProvider);
    final query = ref.watch(peopleQueryProvider);
    final scheme = Theme.of(context).colorScheme;

    return Scaffold(
      appBar: AppBar(
        title: const Text('People'),
        bottom: PreferredSize(
          preferredSize: const Size.fromHeight(64),
          child: Padding(
            padding: const EdgeInsets.fromLTRB(16, 0, 16, 12),
            child: TextField(
              controller: _searchCtrl,
              decoration: InputDecoration(
                hintText: 'Search people, tags…',
                prefixIcon: const Icon(Icons.search),
                suffixIcon: _searchCtrl.text.isNotEmpty
                    ? IconButton(icon: const Icon(Icons.clear), onPressed: () {
                        _searchCtrl.clear();
                        ref.read(peopleQueryProvider.notifier).state = PeopleQuery(group: query.group);
                      })
                    : null,
                filled: true,
                fillColor: scheme.surfaceContainerHighest,
                border: OutlineInputBorder(borderRadius: BorderRadius.circular(14), borderSide: BorderSide.none),
                contentPadding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
              ),
              onChanged: (v) => ref.read(peopleQueryProvider.notifier).state = PeopleQuery(q: v, group: query.group),
              onSubmitted: (v) => ref.read(peopleQueryProvider.notifier).state = PeopleQuery(q: v, group: query.group),
            ),
          ),
        ),
      ),
      body: Column(children: [
        // Group filter chips
        async.when(
          data: (data) {
            if (data.groups.isEmpty) return const SizedBox.shrink();
            return SizedBox(
              height: 44,
              child: ListView(
                scrollDirection: Axis.horizontal,
                padding: const EdgeInsets.symmetric(horizontal: 16),
                children: [
                  ChoiceChip(
                    label: const Text('All'),
                    selected: query.group == null,
                    onSelected: (_) => ref.read(peopleQueryProvider.notifier).state = PeopleQuery(q: query.q),
                  ),
                  const SizedBox(width: 8),
                  ...data.groups.map((g) => Padding(
                        padding: const EdgeInsets.only(right: 8),
                        child: ChoiceChip(
                          label: Text('${g.name} · ${g.memberCount}'),
                          selected: query.group == g.id,
                          onSelected: (_) => ref.read(peopleQueryProvider.notifier).state =
                              PeopleQuery(q: query.q, group: g.id == query.group ? null : g.id),
                        ),
                      )),
                ],
              ),
            );
          },
          loading: () => const SizedBox.shrink(),
          error: (_, _) => const SizedBox.shrink(),
        ),
        // Sync row
        Padding(
          padding: const EdgeInsets.fromLTRB(16, 8, 16, 0),
          child: Row(children: [
            Expanded(
              child: OutlinedButton.icon(
                onPressed: _syncingCardDav ? null : () => _sync('carddav'),
                icon: _syncingCardDav
                    ? const SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2))
                    : const Icon(Icons.contacts, size: 18),
                label: const Text('Sync CardDAV'),
              ),
            ),
            const SizedBox(width: 8),
            Expanded(
              child: OutlinedButton.icon(
                onPressed: _syncingImmich ? null : () => _sync('immich'),
                icon: _syncingImmich
                    ? const SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2))
                    : const Icon(Icons.photo_library, size: 18),
                label: const Text('Sync Immich'),
              ),
            ),
          ]),
        ),
        if (_syncResult != null)
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 8, 16, 0),
            child: Container(
              width: double.infinity,
              padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
              decoration: BoxDecoration(color: scheme.secondaryContainer, borderRadius: BorderRadius.circular(12)),
              child: Text(_syncResult!, style: TextStyle(fontSize: 12, color: scheme.onSecondaryContainer)),
            ),
          ),
        const SizedBox(height: 8),
        Expanded(
          child: async.when(
            loading: () => const LoadingView(),
            error: (e, _) => ErrorView(message: e.toString(), onRetry: () => ref.invalidate(peopleProvider)),
            data: (data) {
              if (data.people.isEmpty) {
                return const EmptyState(
                  icon: Icons.people_outline,
                  title: 'No people found',
                  subtitle: 'Try a different search or group filter.',
                );
              }
              return RefreshIndicator(
                onRefresh: () async => ref.invalidate(peopleProvider),
                child: ListView.separated(
                  padding: const EdgeInsets.fromLTRB(16, 8, 16, 24),
                  itemCount: data.people.length,
                  separatorBuilder: (_, _) => const SizedBox(height: 10),
                  itemBuilder: (context, i) {
                    final p = data.people[i];
                    return Card(
                      child: ListTile(
                        contentPadding: const EdgeInsets.fromLTRB(12, 8, 12, 8),
                        leading: PersonAvatar(photoUrl: p.photoUrl, name: p.name, radius: 24),
                        title: Text(p.name, style: const TextStyle(fontWeight: FontWeight.w700)),
                        subtitle: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                          if (p.tags.isNotEmpty)
                            Padding(
                              padding: const EdgeInsets.only(top: 4),
                              child: Wrap(
                                spacing: 6,
                                children: p.tags
                                    .map((t) => Container(
                                          padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 2),
                                          decoration: BoxDecoration(color: scheme.surfaceContainerHighest, borderRadius: BorderRadius.circular(20)),
                                          child: Text(t, style: TextStyle(fontSize: 11, color: scheme.onSurfaceVariant)),
                                        ))
                                    .toList(),
                              ),
                            ),
                          if (p.nextEvent != null)
                            Padding(
                              padding: const EdgeInsets.only(top: 4),
                              child: Row(children: [
                                Icon(Icons.cake_outlined, size: 12, color: scheme.onSurfaceVariant),
                                const SizedBox(width: 4),
                                Text('${eventTypeLabel(p.nextEvent!.type)} · ${countdownText(p.nextEvent!.daysRemaining)}',
                                    style: TextStyle(fontSize: 12, color: scheme.onSurfaceVariant)),
                              ]),
                            ),
                        ]),
                        trailing: Icon(Icons.chevron_right, color: scheme.outline),
                        onTap: () => context.push('/people/${p.id}'),
                      ),
                    );
                  },
                ),
              );
            },
          ),
        ),
      ]),
    );
  }
}
