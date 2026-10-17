import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:datey/widgets/countdown_badge.dart';

void main() {
  testWidgets('CountdownBadge shows Today for 0', (t) async {
    await t.pumpWidget(const MaterialApp(home: Scaffold(body: CountdownBadge(daysRemaining: 0))));
    expect(find.text('Today'), findsOneWidget);
  });

  testWidgets('CountdownBadge shows In 5 days', (t) async {
    await t.pumpWidget(const MaterialApp(home: Scaffold(body: CountdownBadge(daysRemaining: 5))));
    expect(find.text('In 5 days'), findsOneWidget);
  });

  testWidgets('LunarBadge shows label and leap', (t) async {
    await t.pumpWidget(const MaterialApp(home: Scaffold(body: LunarBadge(label: '九月十五', isLeap: true))));
    expect(find.text('九月十五 · Leap'), findsOneWidget);
  });

  testWidgets('MilestoneBadge shows label', (t) async {
    await t.pumpWidget(const MaterialApp(home: Scaffold(body: MilestoneBadge(label: '30th birthday'))));
    expect(find.text('30th birthday'), findsOneWidget);
  });
}
