import 'package:datey/utils/cookies.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('extracts a cookie from a header with several values', () {
    expect(parseCookieHeader('a=b; session=xyz; theme=dark', 'session'), 'xyz');
  });

  test('extracts the session cookie from a Set-Cookie header', () {
    expect(
      parseCookieHeader('session=abc123; Path=/; HttpOnly; SameSite=Lax', 'session'),
      'abc123',
    );
    expect(parseCookieHeader('Path=/; session=xyz; HttpOnly', 'session'), 'xyz');
  });

  test('ignores similarly named cookies', () {
    expect(parseCookieHeader('session_backup=nope; theme=dark', 'session'), isNull);
    expect(parseCookieHeader('csrf_token=abc; session=xyz', 'session'), 'xyz');
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
