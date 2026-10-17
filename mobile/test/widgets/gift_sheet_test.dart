import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:datey/widgets/gift_sheet.dart';
import 'package:datey/models/gift_idea.dart';

void main() {
  testWidgets('GiftSheet shows title and validates required', (tester) async {
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(
        body: GiftSheet(personId: 1, onSaved: () {}),
      ),
    ));
    expect(find.text('Add gift idea'), findsOneWidget);
    expect(find.text('Title *'), findsOneWidget);
    // Try save without title -> validation
    await tester.tap(find.text('Save gift idea'));
    await tester.pump();
    expect(find.text('Title is required'), findsOneWidget);
  });

  testWidgets('GiftSheet pre-fills existing gift', (tester) async {
    final existing = GiftIdea(
      id: 1,
      title: 'Book',
      notes: 'nice',
      priceCents: 1999,
      url: 'https://example.com',
      status: 'purchased',
      createdAt: '',
      updatedAt: '',
    );
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(body: GiftSheet(personId: 1, existing: existing, onSaved: () {})),
    ));
    expect(find.text('Edit gift idea'), findsOneWidget);
    expect(find.text('Book'), findsOneWidget);
  });

  testWidgets('GiftStatusChip shows label', (tester) async {
    await tester.pumpWidget(const MaterialApp(home: Scaffold(body: GiftStatusChip(status: 'purchased'))));
    expect(find.text('Purchased'), findsOneWidget);
  });
}
