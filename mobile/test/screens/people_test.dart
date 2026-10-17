import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:datey/screens/people_screen.dart';
import 'package:datey/providers/people_provider.dart';
import 'package:datey/models/person_card.dart';

void main() {
  testWidgets('People shows empty then list', (tester) async {
    final state = PeopleState(people: [], groups: [], total: 0);
    await tester.pumpWidget(ProviderScope(
      overrides: [peopleProvider.overrideWith((ref) async => state)],
      child: const MaterialApp(home: PeopleScreen()),
    ));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 100));
    expect(find.text('No people found'), findsOneWidget);
  });

  testWidgets('People shows cards', (tester) async {
    final people = [
      PersonCard(id: 1, name: 'Dana', firstName: 'Dana', lastName: '', photoUrl: null, tags: ['family'], groups: [], nextEvent: null),
      PersonCard(
          id: 2,
          name: 'Alex',
          firstName: 'Alex',
          lastName: '',
          photoUrl: null,
          tags: [],
          groups: [GroupRef(id: 1, name: 'Family')],
          nextEvent: NextEventRef(type: 'birthday', date: '2026-10-20', daysRemaining: 3)),
    ];
    final groups = [Group(id: 1, name: 'Family', description: '', memberCount: 2)];
    final st = PeopleState(people: people, groups: groups, total: 2);
    await tester.pumpWidget(ProviderScope(
      overrides: [peopleProvider.overrideWith((ref) async => st)],
      child: const MaterialApp(home: PeopleScreen()),
    ));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 100));
    expect(find.text('Dana'), findsOneWidget);
    expect(find.text('Alex'), findsOneWidget);
    expect(find.textContaining('Family'), findsWidgets);
  });

  testWidgets('People search field exists', (tester) async {
    final st = PeopleState(people: [], groups: [], total: 0);
    await tester.pumpWidget(ProviderScope(
      overrides: [peopleProvider.overrideWith((ref) async => st)],
      child: const MaterialApp(home: PeopleScreen()),
    ));
    await tester.pump();
    expect(find.byType(TextField), findsWidgets);
  });
}
