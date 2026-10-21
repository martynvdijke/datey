import 'package:dio/dio.dart';
import 'storage_service.dart';

class ApiClient {
  final Dio dio;
  final StorageService storage;

  ApiClient({required this.storage, Dio? dioOverride, String? baseUrl})
      : dio = dioOverride ??
            Dio(BaseOptions(
              baseUrl: baseUrl ?? '',
              connectTimeout: const Duration(seconds: 10),
              receiveTimeout: const Duration(seconds: 15),
              headers: {'Accept': 'application/json'},
              validateStatus: (s) => s != null && s < 500,
            )) {
    if (dioOverride == null) {
      dio.interceptors.add(InterceptorsWrapper(
        onRequest: (options, handler) async {
          // Ensure Accept header
          options.headers['Accept'] = 'application/json';
          final session = await storage.getSession();
          final csrf = await storage.getCsrf();
          if (session != null && session.isNotEmpty) {
            final existing = options.headers['Cookie'] as String?;
            final cookie = [
              'session=$session',
              if (csrf != null && csrf.isNotEmpty) 'csrf_token=$csrf',
            ].join('; ');
            options.headers['Cookie'] = existing == null ? cookie : '$existing; $cookie';
          }
          // CSRF for POST
          if (options.method.toUpperCase() == 'POST' && csrf != null && csrf.isNotEmpty) {
            options.headers['X-CSRF-Token'] = csrf;
          }
          handler.next(options);
        },
        onResponse: (resp, handler) async {
          final setCookies = resp.headers.map['set-cookie'];
          if (setCookies != null && setCookies.isNotEmpty) {
            await storage.persistCookies(setCookies);
          }
          handler.next(resp);
        },
      ));
    }
  }

  String _resolvePhotoUrl(String? photoUrl, String? baseUrl) {
    if (photoUrl == null || photoUrl.isEmpty) return '';
    if (photoUrl.startsWith('http')) return photoUrl;
    final base = baseUrl ?? dio.options.baseUrl;
    if (base.isEmpty) return photoUrl;
    return base.endsWith('/') ? '$base${photoUrl.replaceFirst('/', '')}' : '$base$photoUrl';
  }

  String resolvePhotoUrl(String? photoUrl) => _resolvePhotoUrl(photoUrl, null);

  Future<Response<dynamic>> getMe() => dio.get('/api/me');

  Future<Response<dynamic>> getOidcStatus() => dio.get('/api/auth/oidc/status');

  /// Checks whether a server advertises OIDC support without sending the
  /// stored session cookie. Used while the user is still typing a server URL.
  static Future<bool> oidcEnabledAt(String baseUrl) async {
    try {
      final probe = Dio(BaseOptions(
        baseUrl: baseUrl,
        connectTimeout: const Duration(seconds: 8),
        receiveTimeout: const Duration(seconds: 8),
        headers: {'Accept': 'application/json'},
        validateStatus: (s) => s != null && s < 500,
      ));
      final resp = await probe.get('/api/auth/oidc/status');
      return resp.statusCode == 200 &&
          resp.data is Map &&
          resp.data['enabled'] == true;
    } catch (_) {
      return false;
    }
  }

  Future<Response<dynamic>> login(String username, String password) => dio.post(
        '/login',
        data: {'username': username, 'password': password},
        options: Options(contentType: 'application/json'),
      );

  Future<Response<dynamic>> getDashboard() => dio.get('/');

  Future<Response<dynamic>> getPeople({String? q, int? group, String? tag}) => dio.get(
        '/people',
        queryParameters: {
          if (q != null && q.isNotEmpty) 'q': q,
          // ignore: use_null_aware_elements
          if (group != null) 'group': group,
          if (tag != null && tag.isNotEmpty) 'tag': tag,
        },
      );

  Future<Response<dynamic>> getPerson(int id) => dio.get('/people/$id');

  Future<Response<dynamic>> getGroups() => dio.get('/groups');

  Future<Response<dynamic>> createGift(int personId, Map<String, dynamic> body) => dio.post(
        '/people/$personId/gift-ideas',
        data: body,
        options: Options(contentType: 'application/json'),
      );

  Future<Response<dynamic>> updateGift(int personId, int giftId, Map<String, dynamic> body) => dio.post(
        '/people/$personId/gift-ideas/$giftId',
        data: body,
        options: Options(contentType: 'application/json'),
      );

  Future<Response<dynamic>> updateGiftStatus(int personId, int giftId, String status) => dio.post(
        '/people/$personId/gift-ideas/$giftId/status',
        data: {'status': status},
        options: Options(contentType: 'application/json'),
      );

  Future<Response<dynamic>> deleteGift(int personId, int giftId) => dio.post(
        '/people/$personId/gift-ideas/$giftId/delete',
        options: Options(contentType: 'application/json'),
      );

  Future<Response<dynamic>> linkMe(int personId) => dio.post(
        '/api/me/linked-person',
        data: {'person_id': personId},
        options: Options(contentType: 'application/json'),
      );

  Future<Response<dynamic>> syncCardDav() => dio.post(
        '/settings/carddav/sync',
        options: Options(contentType: 'application/json'),
      );

  Future<Response<dynamic>> syncImmich() => dio.post(
        '/settings/immich/sync',
        options: Options(contentType: 'application/json'),
      );

  /// Test connection by hitting /api/me or /api/auth/oidc/status
  Future<bool> testConnection(String baseUrl) async {
    try {
      final testDio = Dio(BaseOptions(
        baseUrl: baseUrl,
        connectTimeout: const Duration(seconds: 8),
        receiveTimeout: const Duration(seconds: 8),
        headers: {'Accept': 'application/json'},
        validateStatus: (_) => true,
      ));
      final resp = await testDio.get('/api/auth/oidc/status');
      return resp.statusCode != null && resp.statusCode! < 500;
    } catch (_) {
      return false;
    }
  }
}
