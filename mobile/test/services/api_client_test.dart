import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:datey/services/api_client.dart';
import 'package:datey/services/storage_service.dart';
import 'package:datey/utils/formatters.dart' show eventTypeLabel;

class FakeStorageService extends StorageService {
  FakeStorageService() : super(null);
  @override
  Future<String?> getBaseUrl() async => 'https://example.com';
  @override
  Future<String?> getSession() async => 'fake-session';
  @override
  Future<String?> getCsrf() async => 'fake-csrf';
  @override
  Future<void> persistCookies(List<String> setCookieHeaders) async {}
}

void main() {
  group('ApiClient gift endpoints', () {
    test('updateGift sends POST to /people/{id}/gift-ideas/{giftId} with body', () async {
      String? capturedPath;
      String? capturedMethod;
      dynamic capturedData;
      final dio = Dio();
      dio.interceptors.add(InterceptorsWrapper(
        onRequest: (options, handler) {
          capturedPath = options.path;
          capturedMethod = options.method;
          capturedData = options.data;
          handler.resolve(Response(
            requestOptions: options,
            statusCode: 200,
            data: {
              'id': 5,
              'title': 'Updated Book',
              'notes': 'new notes',
              'price_cents': 2500,
              'url': 'https://example.com',
              'status': 'purchased',
              'created_at': '2026-10-17T10:00:00Z',
              'updated_at': '2026-10-17T11:00:00Z',
            },
          ));
        },
      ));
      final storage = FakeStorageService();
      final client = ApiClient(storage: storage, dioOverride: dio, baseUrl: 'https://example.com');

      final resp = await client.updateGift(1, 5, {
        'title': 'Updated Book',
        'notes': 'new notes',
        'price_cents': 2500,
        'url': 'https://example.com',
        'status': 'purchased',
      });

      expect(capturedPath, '/people/1/gift-ideas/5');
      expect(capturedMethod, 'POST');
      expect(capturedData['title'], 'Updated Book');
      expect(capturedData['price_cents'], 2500);
      expect(resp.statusCode, 200);
      expect(resp.data['title'], 'Updated Book');
    });

    test('createGift still works', () async {
      String? capturedPath;
      final dio = Dio();
      dio.interceptors.add(InterceptorsWrapper(onRequest: (o, h) {
        capturedPath = o.path;
        h.resolve(Response(requestOptions: o, statusCode: 201, data: {'id': 10, 'title': 'Book', 'notes': '', 'price_cents': null, 'url': null, 'status': 'idea', 'created_at': '', 'updated_at': ''}));
      }));
      final client = ApiClient(storage: FakeStorageService(), dioOverride: dio, baseUrl: 'https://example.com');
      final resp = await client.createGift(2, {'title': 'Book', 'status': 'idea'});
      expect(capturedPath, '/people/2/gift-ideas');
      expect(resp.statusCode, 201);
    });

    test('deleteGift posts to /delete', () async {
      String? capturedPath;
      final dio = Dio();
      dio.interceptors.add(InterceptorsWrapper(onRequest: (o, h) {
        capturedPath = o.path;
        h.resolve(Response(requestOptions: o, statusCode: 204, data: null));
      }));
      final client = ApiClient(storage: FakeStorageService(), dioOverride: dio, baseUrl: 'https://example.com');
      final resp = await client.deleteGift(3, 7);
      expect(capturedPath, '/people/3/gift-ideas/7/delete');
      expect(resp.statusCode, 204);
    });
  });

  group('ApiClient note endpoints', () {
    test('createPersonNote posts note and date', () async {
      String? capturedPath;
      String? capturedMethod;
      dynamic capturedData;
      final dio = Dio();
      dio.interceptors.add(InterceptorsWrapper(onRequest: (o, h) {
        capturedPath = o.path;
        capturedMethod = o.method;
        capturedData = o.data;
        h.resolve(Response(
          requestOptions: o,
          statusCode: 201,
          data: {
            'id': 9,
            'note': 'Hello',
            'note_date': '2026-01-02',
            'created_at': '2026-01-02T10:00:00Z',
            'updated_at': '2026-01-02T10:00:00Z',
          },
        ));
      }));
      final client = ApiClient(storage: FakeStorageService(), dioOverride: dio, baseUrl: 'https://example.com');
      final resp = await client.createPersonNote(2, note: 'Hello', noteDate: '2026-01-02');
      expect(capturedPath, '/people/2/notes');
      expect(capturedMethod, 'POST');
      expect(capturedData['note'], 'Hello');
      expect(capturedData['note_date'], '2026-01-02');
      expect(resp.statusCode, 201);
    });

    test('createPersonNote omits date when not provided', () async {
      dynamic capturedData;
      final dio = Dio();
      dio.interceptors.add(InterceptorsWrapper(onRequest: (o, h) {
        capturedData = o.data;
        h.resolve(Response(requestOptions: o, statusCode: 201, data: {}));
      }));
      final client = ApiClient(storage: FakeStorageService(), dioOverride: dio, baseUrl: 'https://example.com');
      await client.createPersonNote(2, note: 'Hello');
      expect((capturedData as Map).containsKey('note_date'), false);
    });

    test('deletePersonNote posts to /delete', () async {
      String? capturedPath;
      final dio = Dio();
      dio.interceptors.add(InterceptorsWrapper(onRequest: (o, h) {
        capturedPath = o.path;
        h.resolve(Response(requestOptions: o, statusCode: 204, data: null));
      }));
      final client = ApiClient(storage: FakeStorageService(), dioOverride: dio, baseUrl: 'https://example.com');
      final resp = await client.deletePersonNote(3, 7);
      expect(capturedPath, '/people/3/notes/7/delete');
      expect(resp.statusCode, 204);
    });
  });

  group('account deletion', () {
    test('deleteAccount sends DELETE /api/v1/me with confirmation', () async {
      String? capturedPath;
      String? capturedMethod;
      dynamic capturedData;
      final dio = Dio();
      dio.interceptors.add(InterceptorsWrapper(onRequest: (o, h) {
        capturedPath = o.path;
        capturedMethod = o.method;
        capturedData = o.data;
        h.resolve(Response(requestOptions: o, statusCode: 204, data: null));
      }));
      final client = ApiClient(storage: FakeStorageService(), dioOverride: dio, baseUrl: 'https://example.com');

      await client.deleteAccount('alice');

      expect(capturedPath, '/api/v1/me');
      expect(capturedMethod, 'DELETE');
      expect(capturedData['confirm'], 'alice');
    });

    test('deleteAccount surfaces the server error message', () async {
      final dio = Dio();
      dio.interceptors.add(InterceptorsWrapper(onRequest: (o, h) {
        h.resolve(Response(
          requestOptions: o,
          statusCode: 409,
          data: {'error': 'last_admin', 'message': 'The last admin cannot be deleted'},
        ));
      }));
      final client = ApiClient(storage: FakeStorageService(), dioOverride: dio, baseUrl: 'https://example.com');

      expect(
        () => client.deleteAccount('alice'),
        throwsA(isA<ApiException>().having((e) => e.message, 'message', contains('last admin'))),
      );
    });
  });

  group('account export', () {
    test('exportAccount GETs /api/v1/me/export and returns the JSON map', () async {
      String? capturedPath;
      String? capturedMethod;
      Map<String, dynamic>? capturedHeaders;
      final dio = Dio(BaseOptions(
        baseUrl: 'https://example.com',
        headers: {'Accept': 'application/json'},
      ));
      dio.interceptors.add(InterceptorsWrapper(onRequest: (o, h) {
        capturedPath = o.path;
        capturedMethod = o.method;
        capturedHeaders = Map<String, dynamic>.from(o.headers);
        h.resolve(Response(
          requestOptions: o,
          statusCode: 200,
          headers: Headers.fromMap({
            'content-type': ['application/json'],
          }),
          data: {
            'user': {'username': 'alice', 'email': 'alice@example.com'},
            'people': [],
          },
        ));
      }));
      final client = ApiClient(storage: FakeStorageService(), dioOverride: dio, baseUrl: 'https://example.com');

      final data = await client.exportAccount();

      expect(capturedPath, '/api/v1/me/export');
      expect(capturedMethod, 'GET');
      expect(capturedHeaders?['Accept'], 'application/json');
      expect(data['user']['username'], 'alice');
      expect(data['people'], isEmpty);
    });

    test('exportAccount surfaces the server error message', () async {
      final dio = Dio();
      dio.interceptors.add(InterceptorsWrapper(onRequest: (o, h) {
        h.resolve(Response(
          requestOptions: o,
          statusCode: 403,
          data: {'error': 'forbidden', 'message': 'Export is not allowed'},
        ));
      }));
      final client = ApiClient(storage: FakeStorageService(), dioOverride: dio, baseUrl: 'https://example.com');

      expect(
        () => client.exportAccount(),
        throwsA(isA<ApiException>().having((e) => e.message, 'message', contains('not allowed'))),
      );
    });
  });

  group('authConfigAt', () {
    tearDown(() => ApiClient.authConfigProbeOverride = null);

    test('parses an enabled demo config from /api/auth/config', () async {
      String? capturedPath;
      final dio = Dio(BaseOptions(baseUrl: 'https://example.com'));
      dio.interceptors.add(InterceptorsWrapper(onRequest: (o, h) {
        capturedPath = o.path;
        h.resolve(Response(
          requestOptions: o,
          statusCode: 200,
          data: {
            'demo': {'enabled': true, 'handle': 'demo', 'password': 'demo'},
          },
        ));
      }));

      final config = await ApiClient.authConfigAt('https://example.com/', probeDio: dio);

      expect(capturedPath, '/api/auth/config');
      expect(config?['demo']['enabled'], true);
      expect(config?['demo']['handle'], 'demo');
      expect(config?['demo']['password'], 'demo');
    });

    test('parses a disabled demo config', () async {
      final dio = Dio(BaseOptions(baseUrl: 'https://example.com'));
      dio.interceptors.add(InterceptorsWrapper(onRequest: (o, h) {
        h.resolve(Response(
          requestOptions: o,
          statusCode: 200,
          data: {
            'demo': {'enabled': false},
          },
        ));
      }));

      final config = await ApiClient.authConfigAt('https://example.com', probeDio: dio);

      expect(config?['demo']['enabled'], false);
    });

    test('returns null on HTTP errors', () async {
      final dio = Dio(BaseOptions(baseUrl: 'https://example.com'));
      dio.interceptors.add(InterceptorsWrapper(onRequest: (o, h) {
        h.resolve(Response(requestOptions: o, statusCode: 500, data: {'error': 'boom'}));
      }));

      expect(await ApiClient.authConfigAt('https://example.com', probeDio: dio), isNull);
    });

    test('returns null when the request throws', () async {
      final dio = Dio(BaseOptions(baseUrl: 'https://example.com'));
      dio.interceptors.add(InterceptorsWrapper(onRequest: (o, h) {
        h.reject(DioException(requestOptions: o, message: 'network down'));
      }));

      expect(await ApiClient.authConfigAt('https://example.com', probeDio: dio), isNull);
    });
  });

  group('eventTypeLabel guard', () {
    test('empty type does not throw', () async {
      // Import from real utils
      // ignore: avoid_dynamic_calls
      final label = eventTypeLabel('');
      expect(label, '');
    });
  });
}
