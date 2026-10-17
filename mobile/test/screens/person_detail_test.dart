import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:datey/screens/person_detail_screen.dart';
import 'package:datey/providers/person_detail_provider.dart';
import 'package:datey/models/person_detail.dart';
import 'package:datey/models/gift_idea.dart';

PersonDetail _detail({bool hidden = false, List<GiftIdea>? gifts}) => PersonDetail(
      id: 1,
      name: 'Dana',
      firstName: 'Dana',
      middleName: '',
      lastName: '',
      notes: 'Loves books',
      photoUrl: null,
      immichPersonId: null,
      notifyBirthdays: true,
      timezone: 'UTC',
      tags: ['family'],
      events: [
        PersonEvent(id: 1, type: 'birthday', date: '1995-10-20', description: '', calendarSystem: 'gregorian', lunarLeap: false),
      ],
      giftIdeas: gifts ??
          [
            GiftIdea(id: 1, title: 'Book', notes: 'nice', priceCents: 1999, url: 'https://example.com', status: 'idea', createdAt: '', updatedAt: ''),
          ],
      giftIdeasHidden: hidden,
      relationships: [
        Relationship(id: 1, type: 'partner', label: null, otherPersonId: 2, otherPersonName: 'Alex', direction: 'from'),
      ],
      groups: [],
    );

void main() {
  testWidgets('PersonDetail shows overview', (tester) async {
    await tester.pumpWidget(ProviderScope(
      overrides: [personDetailProvider(1).overrideWith((ref) async => _detail())],
      child: const MaterialApp(home: PersonDetailScreen(personId: 1)),
    ));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 100));
    expect(find.text('Dana'), findsWidgets);
    expect(find.text('Loves books'), findsOneWidget);
  });

  testWidgets('PersonDetail gifts tab shows hidden placeholder', (tester) async {
    await tester.pumpWidget(ProviderScope(
      overrides: [personDetailProvider(1).overrideWith((ref) async => _detail(hidden: true, gifts: []))],
      child: const MaterialApp(home: PersonDetailScreen(personId: 1)),
    ));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 100));
    // Swipe to gifts tab
    await tester.tap(find.text('Gifts'));
    await tester.pumpAndSettle();
    expect(find.text('Hidden — this is your own profile'), findsOneWidget);
  });

  testWidgets('PersonDetail gifts tab shows gift list', (tester) async {
    await tester.pumpWidget(ProviderScope(
      overrides: [personDetailProvider(1).overrideWith((ref) async => _detail(hidden: false))],
      child: const MaterialApp(home: PersonDetailScreen(personId: 1)),
    ));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 100));
    await tester.tap(find.text('Gifts'));
    await tester.pumpAndSettle();
    expect(find.text('Book'), findsOneWidget);
    expect(find.textContaining('19.99'), findsOneWidget);
  });

  testWidgets('PersonDetail relationships tab', (tester) async {
    await tester.pumpWidget(ProviderScope(
      overrides: [personDetailProvider(1).overrideWith((ref) async => _detail())],
      child: const MaterialApp(home: PersonDetailScreen(personId: 1)),
    ));
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 100));
    await tester.tap(find.text('Relationships'));
    await tester.pumpAndSettle();
    expect(find.text('Partner'), findsOneWidget);
    expect(find.textContaining('Alex'), findsOneWidget);
  });
}
