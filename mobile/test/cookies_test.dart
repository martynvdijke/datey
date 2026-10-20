import 'package:datey/utils/cookies.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('extracts a cookie from a header with several values', () {
    expect(parseCookieHeader('a=b; session=xyz; theme=dark', 'session'), 'xyz');
  });

  test('returns null when the cookie is missing', () {
    expect(parseCookieHeader('a=b; theme=dark', 'session'), isNull);
    expect(parseCookieHeader(null, 'session'), isNull);
    expect(parseCookieHeader('', 'session'), isNull);
  });

  test('returns null for an empty cookie value', () {
    expect(parseCookieHeader('session=; theme=dark', 'session'), isNull);
  });
}
