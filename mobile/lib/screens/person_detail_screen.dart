import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:url_launcher/url_launcher.dart';
import '../providers/person_detail_provider.dart';
import '../providers/auth_provider.dart';
import '../models/person_detail.dart';
import '../services/api_client.dart';
import '../services/storage_service.dart';
import '../utils/formatters.dart';
import '../widgets/person_avatar.dart';
import '../widgets/countdown_badge.dart';
import '../widgets/empty_state.dart';
import '../widgets/gift_sheet.dart';

class PersonDetailScreen extends ConsumerStatefulWidget {
  final int personId;
  const PersonDetailScreen({super.key, required this.personId});
  @override
  ConsumerState<PersonDetailScreen> createState() => _PersonDetailScreenState();
}

class _PersonDetailScreenState extends ConsumerState<PersonDetailScreen> with SingleTickerProviderStateMixin {
  late TabController _tab;
  bool _linking = false;

  @override
  void initState() {
    super.initState();
    _tab = TabController(length: 3, vsync: this);
  }

  @override
  void dispose() {
    _tab.dispose();
    super.dispose();
  }

  Future<void> _linkMe() async {
    setState(() => _linking = true);
    try {
      final storage = StorageService();
      final baseUrl = await storage.getBaseUrl();
      if (baseUrl == null) throw Exception('No server URL');
      final client = ApiClient(storage: storage, baseUrl: baseUrl);
      final resp = await client.linkMe(widget.personId);
      if (!mounted) return;
      if (resp.statusCode == 200) {
        await ref.read(authStateProvider.notifier).validateSession();
        if (!mounted) return;
        ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Linked as your profile — gifts for you will be hidden')));
        ref.invalidate(personDetailProvider(widget.personId));
      } else {
        if (!mounted) return;
        final msg = resp.data is Map ? resp.data['message'] as String? ?? 'Failed' : 'Failed';
        ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(msg)));
      }
    } catch (e) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.toString())));
    } finally {
      if (mounted) setState(() => _linking = false);
    }
  }

  Future<void> _deleteGift(int giftId) async {
    final ok = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('Delete gift idea?'),
        content: const Text('This cannot be undone.'),
        actions: [
          TextButton(onPressed: () => Navigator.pop(ctx, false), child: const Text('Cancel')),
          FilledButton(onPressed: () => Navigator.pop(ctx, true), child: const Text('Delete')),
        ],
      ),
    );
    if (ok != true) return;
    try {
      final storage = StorageService();
      final baseUrl = await storage.getBaseUrl();
      if (baseUrl == null) return;
      final client = ApiClient(storage: storage, baseUrl: baseUrl);
      final resp = await client.deleteGift(widget.personId, giftId);
      if (resp.statusCode == 204 || resp.statusCode == 200) {
        ref.invalidate(personDetailProvider(widget.personId));
        if (mounted) ScaffoldMessenger.of(context).showSnackBar(const SnackBar(content: Text('Gift idea deleted')));
      }
    } catch (e) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.toString())));
    }
  }

  Future<void> _quickStatus(int giftId, String status) async {
    try {
      final storage = StorageService();
      final baseUrl = await storage.getBaseUrl();
      if (baseUrl == null) return;
      final client = ApiClient(storage: storage, baseUrl: baseUrl);
      final resp = await client.updateGiftStatus(widget.personId, giftId, status);
      if (resp.statusCode == 200) {
        ref.invalidate(personDetailProvider(widget.personId));
      }
    } catch (e) {
      if (mounted) ScaffoldMessenger.of(context).showSnackBar(SnackBar(content: Text(e.toString())));
    }
  }

  @override
  Widget build(BuildContext context) {
    final async = ref.watch(personDetailProvider(widget.personId));
    final auth = ref.watch(authStateProvider);
    final scheme = Theme.of(context).colorScheme;

    return Scaffold(
      appBar: AppBar(
        leading: IconButton(icon: const Icon(Icons.arrow_back), onPressed: () => context.pop()),
        title: async.when(
          data: (p) => Text(p.name),
          loading: () => const Text(''),
          error: (_, _) => const Text(''),
        ),
        bottom: TabBar(
          controller: _tab,
          tabs: const [
            Tab(text: 'Overview'),
            Tab(text: 'Relationships'),
            Tab(text: 'Gifts'),
          ],
        ),
      ),
      body: async.when(
        loading: () => const LoadingView(),
        error: (e, _) => ErrorView(message: e.toString(), onRetry: () => ref.invalidate(personDetailProvider(widget.personId))),
        data: (p) {
          final isMe = auth.whenOrNull(data: (u) => u?.linkedPersonId == p.id) ?? false;
          return TabBarView(
            controller: _tab,
            children: [
              _overviewTab(context, p, isMe, scheme),
              _relationshipsTab(context, p),
              _giftsTab(context, p, scheme),
            ],
          );
        },
      ),
      floatingActionButton: async.whenOrNull(
            data: (p) => _tab.index == 2
                ? FloatingActionButton.extended(
                    onPressed: p.giftIdeasHidden
                        ? null
                        : () => showModalBottomSheet(
                              context: context,
                              isScrollControlled: true,
                              shape: const RoundedRectangleBorder(borderRadius: BorderRadius.vertical(top: Radius.circular(20))),
                              builder: (_) => GiftSheet(
                                personId: p.id,
                                onSaved: () => ref.invalidate(personDetailProvider(p.id)),
                              ),
                            ),
                    icon: const Icon(Icons.add),
                    label: const Text('Add gift'),
                  )
                : null,
          ),
    );
  }

  Widget _overviewTab(BuildContext context, PersonDetail p, bool isMe, ColorScheme scheme) {
    return ListView(
      padding: const EdgeInsets.fromLTRB(16, 16, 16, 24),
      children: [
        Card(
          child: Padding(
            padding: const EdgeInsets.all(20),
            child: Column(children: [
              PersonAvatar(photoUrl: p.photoUrl, name: p.name, radius: 40),
              const SizedBox(height: 12),
              Text(p.name, style: const TextStyle(fontSize: 22, fontWeight: FontWeight.w900)),
              if (p.tags.isNotEmpty) ...[
                const SizedBox(height: 8),
                Wrap(
                  spacing: 6,
                  children: p.tags
                      .map((t) => Chip(label: Text(t, style: const TextStyle(fontSize: 12)), visualDensity: VisualDensity.compact))
                      .toList(),
                ),
              ],
              if (p.groups.isNotEmpty) ...[
                const SizedBox(height: 8),
                Wrap(
                  spacing: 6,
                  children: p.groups
                      .map((g) => Container(
                            padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
                            decoration: BoxDecoration(color: scheme.primaryContainer, borderRadius: BorderRadius.circular(20)),
                            child: Text(g.name, style: TextStyle(fontSize: 12, color: scheme.onPrimaryContainer, fontWeight: FontWeight.w600)),
                          ))
                      .toList(),
                ),
              ],
              if (p.notes.isNotEmpty) ...[
                const SizedBox(height: 12),
                Text(p.notes, textAlign: TextAlign.center, style: TextStyle(color: scheme.onSurfaceVariant)),
              ],
              const SizedBox(height: 16),
              if (!isMe)
                SizedBox(
                  width: double.infinity,
                  child: OutlinedButton.icon(
                    onPressed: _linking ? null : _linkMe,
                    icon: _linking
                        ? const SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2))
                        : const Icon(Icons.person_pin, size: 18),
                    label: const Text('This is me'),
                  ),
                ),
              if (isMe)
                Container(
                  padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
                  decoration: BoxDecoration(color: scheme.secondaryContainer, borderRadius: BorderRadius.circular(20)),
                  child: Row(mainAxisSize: MainAxisSize.min, children: [
                    Icon(Icons.check_circle, size: 14, color: scheme.onSecondaryContainer),
                    const SizedBox(width: 6),
                    Text('This is your profile', style: TextStyle(fontSize: 12, color: scheme.onSecondaryContainer, fontWeight: FontWeight.w600)),
                  ]),
                ),
            ]),
          ),
        ),
        const SizedBox(height: 16),
        Text('Events', style: TextStyle(fontWeight: FontWeight.w800, color: scheme.onSurface)),
        const SizedBox(height: 8),
        if (p.events.isEmpty)
          Card(child: Padding(padding: const EdgeInsets.all(16), child: Text('No events yet.', style: TextStyle(color: scheme.onSurfaceVariant))))
        else
          ...p.events.map((ev) => Card(
                margin: const EdgeInsets.only(bottom: 8),
                child: ListTile(
                  leading: Container(
                    width: 44,
                    height: 44,
                    decoration: BoxDecoration(color: scheme.surfaceContainerHighest, borderRadius: BorderRadius.circular(12)),
                    child: Icon(ev.type == 'birthday' ? Icons.cake : Icons.favorite, color: scheme.primary),
                  ),
                  title: Text(eventTypeLabel(ev.type), style: const TextStyle(fontWeight: FontWeight.w700)),
                  subtitle: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                    Text(ev.date, style: TextStyle(color: scheme.onSurfaceVariant)),
                    if (ev.description.isNotEmpty) Text(ev.description, style: TextStyle(fontSize: 12, color: scheme.onSurfaceVariant)),
                    if (ev.calendarSystem == 'lunar')
                      Padding(
                        padding: const EdgeInsets.only(top: 4),
                        child: LunarBadge(label: 'Lunar ${ev.lunarMonth}/${ev.lunarDay}', isLeap: ev.lunarLeap),
                      ),
                  ]),
                ),
              )),
      ],
    );
  }

  Widget _relationshipsTab(BuildContext context, PersonDetail p) {
    final scheme = Theme.of(context).colorScheme;
    if (p.relationships.isEmpty) {
      return const EmptyState(icon: Icons.people_outline, title: 'No relationships', subtitle: 'Add relationships on the web to see them here.');
    }
    return ListView.separated(
      padding: const EdgeInsets.fromLTRB(16, 16, 16, 24),
      itemCount: p.relationships.length,
      separatorBuilder: (_, _) => const SizedBox(height: 8),
      itemBuilder: (context, i) {
        final r = p.relationships[i];
        final isFrom = r.direction == 'from';
        return Card(
          child: ListTile(
            leading: Container(
              width: 40,
              height: 40,
              decoration: BoxDecoration(color: scheme.surfaceContainerHighest, shape: BoxShape.circle),
              child: Icon(isFrom ? Icons.arrow_forward : Icons.arrow_back, size: 18, color: scheme.primary),
            ),
            title: Text(r.displayLabel(), style: const TextStyle(fontWeight: FontWeight.w700)),
            subtitle: Text('${r.displayLabel()} · ${r.otherPersonName}', style: TextStyle(color: scheme.onSurfaceVariant)),
            trailing: Icon(Icons.chevron_right, color: scheme.outline),
            onTap: () => context.push('/people/${r.otherPersonId}'),
          ),
        );
      },
    );
  }

  Widget _giftsTab(BuildContext context, PersonDetail p, ColorScheme scheme) {
    if (p.giftIdeasHidden) {
      return Center(
        child: Padding(
          padding: const EdgeInsets.all(32),
          child: Column(mainAxisSize: MainAxisSize.min, children: [
            Container(
              width: 72,
              height: 72,
              decoration: BoxDecoration(color: scheme.surfaceContainerHighest, shape: BoxShape.circle),
              child: Icon(Icons.visibility_off, size: 32, color: scheme.onSurfaceVariant),
            ),
            const SizedBox(height: 16),
            Text('Hidden — this is your own profile', style: TextStyle(fontSize: 16, fontWeight: FontWeight.w800, color: scheme.onSurface)),
            const SizedBox(height: 8),
            Text('Gift ideas for your own profile are hidden to keep surprises safe. View them on the web if needed.',
                textAlign: TextAlign.center, style: TextStyle(color: scheme.onSurfaceVariant, height: 1.4)),
          ]),
        ),
      );
    }
    if (p.giftIdeas.isEmpty) {
      return EmptyState(
        icon: Icons.card_giftcard,
        title: 'No gift ideas yet',
        subtitle: 'Tap “Add gift” to save something thoughtful.',
        action: FilledButton.icon(
          onPressed: () => showModalBottomSheet(
            context: context,
            isScrollControlled: true,
            shape: const RoundedRectangleBorder(borderRadius: BorderRadius.vertical(top: Radius.circular(20))),
            builder: (_) => GiftSheet(personId: p.id, onSaved: () => ref.invalidate(personDetailProvider(p.id))),
          ),
          icon: const Icon(Icons.add),
          label: const Text('Add gift idea'),
        ),
      );
    }
    return ListView.separated(
      padding: const EdgeInsets.fromLTRB(16, 16, 16, 80),
      itemCount: p.giftIdeas.length,
      separatorBuilder: (_, _) => const SizedBox(height: 10),
      itemBuilder: (context, i) {
        final g = p.giftIdeas[i];
        return Card(
          child: Padding(
            padding: const EdgeInsets.all(14),
            child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
              Row(children: [
                Expanded(child: Text(g.title, style: const TextStyle(fontWeight: FontWeight.w800, fontSize: 15))),
                GiftStatusChip(status: g.status),
              ]),
              if (g.notes.isNotEmpty) ...[
                const SizedBox(height: 6),
                Text(g.notes, style: TextStyle(color: scheme.onSurfaceVariant, fontSize: 13)),
              ],
              const SizedBox(height: 8),
              Wrap(spacing: 8, runSpacing: 6, children: [
                if (g.priceCents != null)
                  Container(
                    padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
                    decoration: BoxDecoration(color: scheme.surfaceContainerHighest, borderRadius: BorderRadius.circular(20)),
                    child: Text(priceLabel(g.priceCents), style: TextStyle(fontSize: 12, fontWeight: FontWeight.w600, color: scheme.onSurfaceVariant)),
                  ),
                if (g.url != null && g.url!.isNotEmpty)
                  ActionChip(
                    label: const Text('Open link', style: TextStyle(fontSize: 12)),
                    avatar: const Icon(Icons.link, size: 14),
                    onPressed: () async {
                      final uri = Uri.tryParse(g.url!);
                      if (uri != null) await launchUrl(uri, mode: LaunchMode.externalApplication);
                    },
                  ),
              ]),
              const SizedBox(height: 10),
              Row(children: [
                Expanded(
                  child: Wrap(spacing: 6, children: [
                    _statusAction('idea', g.status, () => _quickStatus(g.id, 'idea')),
                    _statusAction('purchased', g.status, () => _quickStatus(g.id, 'purchased')),
                    _statusAction('given', g.status, () => _quickStatus(g.id, 'given')),
                    _statusAction('archived', g.status, () => _quickStatus(g.id, 'archived')),
                  ]),
                ),
                IconButton(
                  icon: const Icon(Icons.edit_outlined, size: 18),
                  onPressed: () => showModalBottomSheet(
                    context: context,
                    isScrollControlled: true,
                    shape: const RoundedRectangleBorder(borderRadius: BorderRadius.vertical(top: Radius.circular(20))),
                    builder: (_) => GiftSheet(personId: p.id, existing: g, onSaved: () => ref.invalidate(personDetailProvider(p.id))),
                  ),
                  tooltip: 'Edit',
                ),
                IconButton(
                  icon: Icon(Icons.delete_outline, size: 18, color: scheme.error),
                  onPressed: () => _deleteGift(g.id),
                  tooltip: 'Delete',
                ),
              ]),
            ]),
          ),
        );
      },
    );
  }

  Widget _statusAction(String target, String current, VoidCallback onTap) {
    final isActive = target == current;
    return ChoiceChip(
      label: Text(giftStatusLabel(target), style: const TextStyle(fontSize: 11)),
      selected: isActive,
      onSelected: (_) {
        if (!isActive) onTap();
      },
      visualDensity: VisualDensity.compact,
      materialTapTargetSize: MaterialTapTargetSize.shrinkWrap,
    );
  }
}
