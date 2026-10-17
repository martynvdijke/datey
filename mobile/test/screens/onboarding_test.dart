import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:datey/screens/onboarding_screen.dart';

void main() {
  testWidgets('Onboarding shows server URL and sign in fields', (tester) async {
    await tester.pumpWidget(const ProviderScope(child: MaterialApp(home: OnboardingScreen())));
    await tester.pump();
    expect(find.text('Welcome to Datey'), findsOneWidget);
    expect(find.text('Server URL'), findsOneWidget);
    expect(find.text('Sign in'), findsWidgets);
    expect(find.text('Username'), findsOneWidget);
    expect(find.text('Password'), findsOneWidget);
    expect(find.text('Test connection'), findsOneWidget);
  });

  testWidgets('Onboarding validates empty server URL on login', (tester) async {
    await tester.pumpWidget(const ProviderScope(child: MaterialApp(home: OnboardingScreen())));
    await tester.pump();
    // Find the FilledButton (Sign in)
    final signInBtn = find.widgetWithText(FilledButton, 'Sign in');
    expect(signInBtn, findsOneWidget);
    await tester.ensureVisible(signInBtn);
    await tester.pumpAndSettle();
    await tester.tap(signInBtn);
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 100));
    expect(find.textContaining('Enter your server URL'), findsOneWidget);
  });
}
