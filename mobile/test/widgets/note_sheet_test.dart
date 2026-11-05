import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:datey/widgets/note_sheet.dart';

void main() {
  testWidgets('NoteSheet validates required note and defaults to today', (tester) async {
    await tester.pumpWidget(MaterialApp(
      home: Scaffold(body: NoteSheet(personId: 1, onSaved: () {})),
    ));
    expect(find.text('Add note'), findsOneWidget);
    expect(find.text('Note *'), findsOneWidget);
    // Date field defaults to today.
    final today = DateTime.now();
    final expected = '${today.year.toString().padLeft(4, '0')}-'
        '${today.month.toString().padLeft(2, '0')}-'
        '${today.day.toString().padLeft(2, '0')}';
    expect(find.text(expected), findsOneWidget);
    // Save without a note -> validation.
    await tester.tap(find.text('Save note'));
    await tester.pump();
    expect(find.text('Note is required'), findsOneWidget);
  });
}
