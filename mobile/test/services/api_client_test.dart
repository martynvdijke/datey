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

  group('eventTypeLabel guard', () {
    test('empty type does not throw', () async {
      // Import from real utils
      // ignore: avoid_dynamic_calls
      final label = eventTypeLabel('');
      expect(label, '');
    });
  });
}
