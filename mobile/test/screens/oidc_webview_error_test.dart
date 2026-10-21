import 'package:datey/screens/oidc_webview_screen.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  group('shouldIgnoreWebResourceError', () {
    test('ignores aborted redirects and cancelled loads', () {
      expect(
        shouldIgnoreWebResourceError(
          errorCode: -3,
          description: 'net::ERR_ABORTED',
        ),
        isTrue,
      );
      expect(
        shouldIgnoreWebResourceError(
          errorCode: -999,
          description: 'Operation was canceled',
        ),
        isTrue,
      );
      expect(
        shouldIgnoreWebResourceError(
          errorCode: -999,
          description: 'The operation was cancelled',
        ),
        isTrue,
      );
      expect(
        shouldIgnoreWebResourceError(
          errorCode: 0,
          description: 'something',
          errorTypeName: 'cancelled',
        ),
        isTrue,
      );
    });

    test('ignores unsupported custom URL schemes', () {
      expect(
        shouldIgnoreWebResourceError(
          errorCode: -10,
          description: 'net::ERR_UNKNOWN_URL_SCHEME',
        ),
        isTrue,
      );
      expect(
        shouldIgnoreWebResourceError(
          errorCode: 0,
          description: 'net::ERR_UNKNOWN_URL_SCHEME',
        ),
        isTrue,
      );
    });

    test('keeps genuine failures', () {
      expect(
        shouldIgnoreWebResourceError(
          errorCode: -2,
          description: 'net::ERR_NAME_NOT_RESOLVED',
        ),
        isFalse,
      );
      expect(
        shouldIgnoreWebResourceError(
          errorCode: -7,
          description: 'net::ERR_TIMED_OUT',
        ),
        isFalse,
      );
      expect(
        shouldIgnoreWebResourceError(
          errorCode: 0,
          description: 'The sign-in provider returned HTTP 403.',
        ),
        isFalse,
      );
    });
  });
}
