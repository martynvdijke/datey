import 'package:flutter_test/flutter_test.dart';
import 'package:datey/utils/validators.dart';

void main() {
  group('validateServerUrl', () {
    test('empty', () => expect(validateServerUrl(''), isNotNull));
    test('null', () => expect(validateServerUrl(null), isNotNull));
    test('no scheme', () => expect(validateServerUrl('example.com'), isNotNull));
    test('http ok', () => expect(validateServerUrl('http://example.com'), isNull));
    test('https ok', () => expect(validateServerUrl('https://datey.example.com'), isNull));
    test('ftp invalid', () => expect(validateServerUrl('ftp://example.com'), isNotNull));
  });

  group('normalizeServerUrl', () {
    test('trims and removes trailing slash', () {
      expect(normalizeServerUrl('https://example.com/'), 'https://example.com');
      expect(normalizeServerUrl('https://example.com///'), 'https://example.com');
      expect(normalizeServerUrl('  https://example.com  '), 'https://example.com');
    });
  });
}
