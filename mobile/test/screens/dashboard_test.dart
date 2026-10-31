import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:datey/screens/dashboard_screen.dart';
import 'package:datey/providers/dashboard_provider.dart';
import 'package:datey/models/upcoming_event.dart';

UpcomingEvent _ev(int id, String name, int days, {String calendar = 'gregorian', String? lunar, bool milestone = false}) =>
    UpcomingEvent(
      id: id,
      personId: id,
      personName: name,
      personPhotoUrl: null,
      type: 'birthday',
      description: '',
      date: '2026-10-20',
      daysRemaining: days,
      turningAge: 30,
      yearsTogether: null,
      calendarSystem: calendar,
      lunarLabel: lunar,
      isLeapMonth: false,
      isMilestone: milestone,
      milestoneLabel: milestone ? '30th birthday' : null,
    );

void main() {
  testWidgets('Dashboard shows empty state', (tester) async {
    await tester.pumpWidget(ProviderScope(
      overrides: [dashboardProvider.overrideWith((ref) async => const DashboardData(today: [], thisWeek: [], thisMonth: [], later: []))],
      child: const MaterialApp(home: DashboardScreen()),
    ));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 100));
    expect(find.text('All caught up'), findsOneWidget);
  });

  testWidgets('Dashboard shows events grouped', (tester) async {
    final data = DashboardData(
      today: [_ev(1, 'Alice', 0, milestone: true)],
      thisWeek: [_ev(2, 'Bob', 3)],
      thisMonth: [_ev(3, 'Carol', 15, calendar: 'lunar', lunar: '九月十五')],
      later: [],
    );
    await tester.pumpWidget(ProviderScope(
      overrides: [dashboardProvider.overrideWith((ref) async => data)],
      child: const MaterialApp(home: DashboardScreen()),
    ));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 100));
    expect(find.text('Today'), findsWidgets);
    expect(find.text('Alice'), findsOneWidget);
    expect(find.text('Bob'), findsOneWidget);
    expect(find.text('30th birthday'), findsOneWidget);
    expect(find.text('九月十五'), findsOneWidget);
  });

  testWidgets('Dashboard error state', (tester) async {
    await tester.pumpWidget(ProviderScope(
      retry: (retryCount, error) => null,
      overrides: [dashboardProvider.overrideWith((ref) async => throw Exception('network error'))],
      child: const MaterialApp(home: DashboardScreen()),
    ));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 100));
    expect(find.textContaining('network error'), findsOneWidget);
  });
}
