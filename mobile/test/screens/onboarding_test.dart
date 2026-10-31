import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:datey/screens/onboarding_screen.dart';
import 'package:datey/services/storage_service.dart';

Future<void> _pumpOnboarding(WidgetTester tester) async {
  await tester.pumpWidget(const ProviderScope(child: MaterialApp(home: OnboardingScreen())));
  // Let the async stored-URL lookup and the OIDC probe complete.
  await tester.pump();
  await tester.pump(const Duration(milliseconds: 100));
}

void main() {
  testWidgets('Onboarding prefills the default server URL on first run', (tester) async {
    FlutterSecureStorage.setMockInitialValues({});
    await _pumpOnboarding(tester);

    final urlField = tester.widget<TextFormField>(find.byType(TextFormField).first);
    expect(urlField.controller?.text, defaultBaseUrl);
    expect(find.text('Use the default instance below, or enter another self-hosted Datey server.'), findsOneWidget);
    // SSO is not offered until a valid server reports that it is enabled.
    expect(find.text('Continue with SSO'), findsNothing);
  });

  testWidgets('Onboarding keeps a previously saved server URL', (tester) async {
    const savedUrl = 'https://birthdays.example.com';
    FlutterSecureStorage.setMockInitialValues({'datey_base_url': savedUrl});
    await _pumpOnboarding(tester);

    final urlField = tester.widget<TextFormField>(find.byType(TextFormField).first);
    expect(urlField.controller?.text, savedUrl);
  });

  testWidgets('Onboarding shows server URL and sign in fields', (tester) async {
    FlutterSecureStorage.setMockInitialValues({});
    await _pumpOnboarding(tester);

    expect(find.text('Welcome to Datey'), findsOneWidget);
    expect(find.text('Server URL'), findsOneWidget);
    expect(find.text('Sign in'), findsWidgets);
    expect(find.text('Username'), findsOneWidget);
    expect(find.text('Password'), findsOneWidget);
    expect(find.text('Test connection'), findsOneWidget);
  });

  testWidgets('Onboarding validates empty server URL on login', (tester) async {
    FlutterSecureStorage.setMockInitialValues({});
    await _pumpOnboarding(tester);

    await tester.enterText(find.byType(TextFormField).first, '');
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
