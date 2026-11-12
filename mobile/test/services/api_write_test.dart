import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:datey/services/api_client.dart';
import 'package:datey/services/storage_service.dart';

class _FakeStorage extends StorageService {
  _FakeStorage() : super(null);
  @override
  Future<String?> getBaseUrl() async => 'https://example.com';
  @override
  Future<String?> getSession() async => 'fake-session';
  @override
  Future<String?> getCsrf() async => 'fake-csrf';
  @override
  Future<void> persistCookies(List<String> setCookieHeaders) async {}
}

/// Records the outgoing request and answers with [status]/[data].
class _Stub {
  String? path;
  String? method;
  dynamic data;

  ApiClient clientWith(int status, dynamic responseData) {
    final dio = Dio();
    dio.interceptors.add(InterceptorsWrapper(
      onRequest: (options, handler) {
        path = options.path;
        method = options.method;
        data = options.data;
        handler.resolve(Response(
          requestOptions: options,
          statusCode: status,
          data: responseData,
        ));
      },
    ));
    return ApiClient(storage: _FakeStorage(), dioOverride: dio, baseUrl: 'https://example.com');
  }
}

void main() {
  group('write API person calls', () {
    test('createPerson POSTs to /api/v1/people', () async {
      final stub = _Stub();
      final client = stub.clientWith(201, {'id': 3, 'name': 'Ada Lovelace'});

      final result = await client.createPerson({'first_name': 'Ada', 'last_name': 'Lovelace'});

      expect(stub.path, '/api/v1/people');
      expect(stub.method, 'POST');
      expect(stub.data['first_name'], 'Ada');
      expect(result['id'], 3);
    });

    test('updatePerson PATCHes /api/v1/people/{id}', () async {
      final stub = _Stub();
      final client = stub.clientWith(200, {'id': 3, 'name': 'Ada'});

      await client.updatePerson(3, {'notes': 'x'});

      expect(stub.path, '/api/v1/people/3');
      expect(stub.method, 'PATCH');
    });

    test('deletePerson accepts 204', () async {
      final stub = _Stub();
      final client = stub.clientWith(204, null);

      await client.deletePerson(3);

      expect(stub.path, '/api/v1/people/3');
      expect(stub.method, 'DELETE');
    });
  });

  group('write API event calls', () {
    test('listPersonEvents reads the events envelope', () async {
      final stub = _Stub();
      final client = stub.clientWith(200, {
        'person_id': 7,
        'events': [
          {'id': 1, 'type': 'birthday', 'date': '1990-05-12', 'frequency': 'monthly', 'interval': 1},
        ],
      });

      final events = await client.listPersonEvents(7);

      expect(stub.path, '/api/v1/people/7/events');
      expect(events, hasLength(1));
      expect(events.first['frequency'], 'monthly');
    });

    test('createEvent POSTs to the person events route', () async {
      final stub = _Stub();
      final client = stub.clientWith(201, {'id': 9, 'type': 'anniversary', 'date': '2026-07-04'});

      final result = await client.createEvent(7, {'type': 'anniversary', 'date': '2026-07-04'});

      expect(stub.path, '/api/v1/people/7/events');
      expect(stub.method, 'POST');
      expect(result['id'], 9);
    });

    test('updateEvent PATCHes /api/v1/events/{id}', () async {
      final stub = _Stub();
      final client = stub.clientWith(200, {'id': 9, 'type': 'custom', 'date': '2026-07-04'});

      await client.updateEvent(9, {'frequency': 'monthly'});

      expect(stub.path, '/api/v1/events/9');
      expect(stub.method, 'PATCH');
    });

    test('deleteEvent accepts 204', () async {
      final stub = _Stub();
      final client = stub.clientWith(204, null);

      await client.deleteEvent(9);

      expect(stub.path, '/api/v1/events/9');
      expect(stub.method, 'DELETE');
    });
  });

  group('write API error mapping', () {
    test('validation envelope becomes ApiValidationException', () async {
      final stub = _Stub();
      final client = stub.clientWith(400, {
        'error': 'validation',
        'message': 'Validation failed',
        'fields': {'name': 'required', 'date': 'date_invalid'},
      });

      await expectLater(
        client.createPerson({}),
        throwsA(isA<ApiValidationException>()
            .having((e) => e.fields['name'], 'name code', 'required')
            .having((e) => e.fields['date'], 'date code', 'date_invalid')),
      );
    });

    test('conflict message is surfaced verbatim', () async {
      final stub = _Stub();
      final client = stub.clientWith(409, {
        'error': 'conflict',
        'message': 'A person with this name already exists',
      });

      await expectLater(
        client.createPerson({'name': 'Ada'}),
        throwsA(isA<ApiException>()
            .having((e) => e.message, 'message', 'A person with this name already exists')),
      );
    });

    test('404 produces a safe exception', () async {
      final stub = _Stub();
      final client = stub.clientWith(404, {'error': 'not_found', 'message': 'Event not found'});

      await expectLater(
        client.deleteEvent(1),
        throwsA(isA<ApiException>().having((e) => e.message, 'message', 'Event not found')),
      );
    });
  });
}
