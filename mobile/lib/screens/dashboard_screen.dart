import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import '../providers/dashboard_provider.dart';
import '../models/upcoming_event.dart';
import '../utils/formatters.dart';
import '../widgets/countdown_badge.dart';
import '../widgets/person_avatar.dart';
import '../widgets/empty_state.dart';

class DashboardScreen extends ConsumerWidget {
  const DashboardScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final async = ref.watch(dashboardProvider);
    return Scaffold(
      appBar: AppBar(
        title: const Text('Upcoming'),
        actions: [
          IconButton(
            icon: const Icon(Icons.refresh),
            onPressed: () => ref.invalidate(dashboardProvider),
            tooltip: 'Refresh',
          ),
        ],
      ),
      body: async.when(
        loading: () => const LoadingView(),
        error: (e, _) => ErrorView(message: e.toString(), onRetry: () => ref.invalidate(dashboardProvider)),
        data: (data) {
          if (data.isEmpty) {
            return RefreshIndicator(
              onRefresh: () async => ref.invalidate(dashboardProvider),
              child: ListView(
                children: const [
                  SizedBox(height: 80),
                  EmptyState(
                    icon: Icons.celebration_outlined,
                    title: 'All caught up',
                    subtitle: 'No upcoming birthdays or anniversaries.\nAdd people to see them here.',
                  ),
                ],
              ),
            );
          }
          return RefreshIndicator(
            onRefresh: () async => ref.invalidate(dashboardProvider),
            child: ListView(
              padding: const EdgeInsets.fromLTRB(16, 8, 16, 24),
              children: [
                _bucket(context, 'today', data.today, ref),
                _bucket(context, 'this_week', data.thisWeek, ref),
                _bucket(context, 'this_month', data.thisMonth, ref),
                _bucket(context, 'later', data.later, ref),
              ],
            ),
          );
        },
      ),
    );
  }

  Widget _bucket(BuildContext context, String key, List<UpcomingEvent> events, WidgetRef ref) {
    if (events.isEmpty) return const SizedBox.shrink();
    final scheme = Theme.of(context).colorScheme;
    return Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
      Padding(
        padding: const EdgeInsets.fromLTRB(4, 16, 4, 10),
        child: Row(children: [
          Text(bucketTitle(key), style: TextStyle(fontSize: 13, fontWeight: FontWeight.w800, letterSpacing: 0.6, color: scheme.onSurfaceVariant)),
          const SizedBox(width: 8),
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
            decoration: BoxDecoration(color: scheme.surfaceContainerHighest, borderRadius: BorderRadius.circular(20)),
            child: Text('${events.length}', style: TextStyle(fontSize: 12, fontWeight: FontWeight.w700, color: scheme.onSurfaceVariant)),
          ),
        ]),
      ),
      ...events.map((e) => _eventCard(context, e)),
    ]);
  }

  Widget _eventCard(BuildContext context, UpcomingEvent e) {
    final scheme = Theme.of(context).colorScheme;
    return Card(
      margin: const EdgeInsets.only(bottom: 12),
      child: InkWell(
        borderRadius: BorderRadius.circular(20),
        onTap: () => context.push('/people/${e.personId}'),
        child: Padding(
          padding: const EdgeInsets.all(14),
          child: Row(children: [
            PersonAvatar(photoUrl: e.personPhotoUrl, name: e.personName, radius: 26),
            const SizedBox(width: 14),
            Expanded(
              child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
                Row(children: [
                  Expanded(
                    child: Text(e.personName, style: const TextStyle(fontWeight: FontWeight.w800, fontSize: 15)),
                  ),
                  CountdownBadge(daysRemaining: e.daysRemaining),
                ]),
                const SizedBox(height: 4),
                Row(children: [
                  Container(
                    padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
                    decoration: BoxDecoration(color: scheme.surfaceContainerHighest, borderRadius: BorderRadius.circular(20)),
                    child: Text(eventTypeLabel(e.type), style: TextStyle(fontSize: 11, fontWeight: FontWeight.w600, color: scheme.onSurfaceVariant)),
                  ),
                  if (turningAgeLabel(e.turningAge, e.type) != null) ...[
                    const SizedBox(width: 6),
                    Text(turningAgeLabel(e.turningAge, e.type)!, style: TextStyle(fontSize: 12, color: scheme.onSurfaceVariant)),
                  ],
                  if (yearsTogetherLabel(e.yearsTogether, e.type) != null) ...[
                    const SizedBox(width: 6),
                    Text(yearsTogetherLabel(e.yearsTogether, e.type)!, style: TextStyle(fontSize: 12, color: scheme.onSurfaceVariant)),
                  ],
                ]),
                if (e.isMilestone && e.milestoneLabel != null) ...[
                  const SizedBox(height: 6),
                  MilestoneBadge(label: e.milestoneLabel!),
                ],
                if (e.calendarSystem == 'lunar' && e.lunarLabel != null) ...[
                  const SizedBox(height: 6),
                  LunarBadge(label: e.lunarLabel!, isLeap: e.isLeapMonth),
                ],
                if (e.description.isNotEmpty) ...[
                  const SizedBox(height: 4),
                  Text(e.description, style: TextStyle(fontSize: 12, color: scheme.onSurfaceVariant)),
                ],
              ]),
            ),
            const SizedBox(width: 8),
            Icon(Icons.chevron_right, color: scheme.outline),
          ]),
        ),
      ),
    );
  }
}
