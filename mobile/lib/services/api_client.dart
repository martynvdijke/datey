import 'package:dio/dio.dart';
import 'package:flutter/foundation.dart';
import 'storage_service.dart';

/// Error returned by an API call whose message is safe to show to the user.
class ApiException implements Exception {
  ApiException(this.message);
  final String message;

  @override
  String toString() => message;
}

/// Validation failure with per-field codes from the versioned write API.
///
/// `fields` maps a request field name to a machine-readable code (for example
/// `name` -> `required`); the UI turns those into inline messages via
/// `apiFieldMessages` in `utils/api_errors.dart`.
class ApiValidationException extends ApiException {
  ApiValidationException(this.fields, {String? message})
      : super(message ?? 'Please check the highlighted fields.');
  final Map<String, String> fields;
}

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
          // CSRF for state-changing requests.
          final method = options.method.toUpperCase();
          if ((method == 'POST' || method == 'PUT' || method == 'PATCH' || method == 'DELETE') &&
              csrf != null &&
              csrf.isNotEmpty) {
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

  Future<Response<dynamic>> getMe() => dio.get('/api/v1/me');

  /// Registers this device's FCM token for the signed-in user.
  Future<void> registerDevice(String token, {String platform = 'android'}) async {
    final resp = await dio.post(
      '/api/v1/devices',
      data: {'token': token, 'platform': platform},
      options: Options(contentType: 'application/json'),
    );
    if (resp.statusCode == 201 || resp.statusCode == 200) return;
    throw _apiError(resp, 'Could not register the device for notifications.');
  }

  /// Removes this device's FCM token for the signed-in user.
  Future<void> unregisterDevice(String token) async {
    final resp = await dio.delete(
      '/api/v1/devices',
      data: {'token': token},
      options: Options(contentType: 'application/json'),
    );
    if (resp.statusCode == 204 || resp.statusCode == 200) return;
    throw _apiError(resp, 'Could not unregister the device for notifications.');
  }

  Future<Response<dynamic>> getOidcStatus() => dio.get('/api/auth/oidc/status');

  /// Download the requester's attributable account data as JSON.
  Future<Map<String, dynamic>> exportAccount() async {
    final resp = await dio.get('/api/v1/me/export');
    if (resp.statusCode == 200 && resp.data is Map) {
      return Map<String, dynamic>.from(resp.data as Map);
    }
    throw ApiException(_errorMessage(resp, 'Could not export the account data.'));
  }

  /// Close the account. The server requires an explicit confirmation matching
  /// the username or the literal DELETE.
  Future<void> deleteAccount(String confirm) async {
    final resp = await dio.delete(
      '/api/v1/me',
      data: {'confirm': confirm},
      options: Options(contentType: 'application/json'),
    );
    if (resp.statusCode == 204 || resp.statusCode == 200) return;
    throw ApiException(_errorMessage(resp, 'Could not delete the account.'));
  }

  /// Builds a typed exception from the write API error envelope:
  /// `{"error": "...", "message": "...", "fields": {"name": "required"}}`.
  ApiException _apiError(Response<dynamic> resp, String fallback) {
    final data = resp.data;
    if (data is Map) {
      final rawFields = data['fields'];
      if (rawFields is Map) {
        final fields = <String, String>{};
        rawFields.forEach((key, value) {
          if (key is String && value is String) fields[key] = value;
        });
        if (fields.isNotEmpty) {
          return ApiValidationException(fields);
        }
      }
      final message = data['message'];
      if (message is String && message.isNotEmpty) return ApiException(message);
    }
    return ApiException(fallback);
  }

  String _errorMessage(Response<dynamic> resp, String fallback) {
    final data = resp.data;
    if (data is Map) {
      final message = data['message'];
      if (message is String && message.isNotEmpty) return message;
    }
    return fallback;
  }

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

  /// Test seam so widget tests can stub the unauthenticated auth-config probe.
  @visibleForTesting
  static Future<Map<String, dynamic>?> Function(String baseUrl)? authConfigProbeOverride;

  /// Reads the public auth configuration (including demo-account details)
  /// without sending the stored session cookie. Used while the user is still
  /// typing a server URL. Returns null when the server cannot be reached or
  /// does not expose a usable config.
  static Future<Map<String, dynamic>?> authConfigAt(String baseUrl, {Dio? probeDio}) async {
    final override = authConfigProbeOverride;
    if (override != null) return override(baseUrl);
    try {
      final probe = probeDio ??
          Dio(BaseOptions(
            baseUrl: _normalizeProbeUrl(baseUrl),
            connectTimeout: const Duration(seconds: 4),
            receiveTimeout: const Duration(seconds: 4),
            headers: {'Accept': 'application/json'},
            validateStatus: (s) => s != null && s < 500,
          ));
      final resp = await probe.get('/api/auth/config');
      if (resp.statusCode == 200 && resp.data is Map) {
        return Map<String, dynamic>.from(resp.data as Map);
      }
      return null;
    } catch (_) {
      return null;
    }
  }

  static String _normalizeProbeUrl(String baseUrl) {
    var v = baseUrl.trim();
    while (v.endsWith('/')) {
      v = v.substring(0, v.length - 1);
    }
    return v;
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

  // ── Versioned write API (people + events) ──────────────────────────────

  /// Creates a person and returns the created record.
  Future<Map<String, dynamic>> createPerson(Map<String, dynamic> body) async {
    final resp = await dio.post(
      '/api/v1/people',
      data: body,
      options: Options(contentType: 'application/json'),
    );
    if (resp.statusCode == 201 && resp.data is Map) {
      return Map<String, dynamic>.from(resp.data as Map);
    }
    throw _apiError(resp, 'Could not create the person.');
  }

  /// Applies a partial update to a person and returns the updated record.
  Future<Map<String, dynamic>> updatePerson(int id, Map<String, dynamic> body) async {
    final resp = await dio.patch(
      '/api/v1/people/$id',
      data: body,
      options: Options(contentType: 'application/json'),
    );
    if (resp.statusCode == 200 && resp.data is Map) {
      return Map<String, dynamic>.from(resp.data as Map);
    }
    throw _apiError(resp, 'Could not update the person.');
  }

  Future<void> deletePerson(int id) async {
    final resp = await dio.delete('/api/v1/people/$id');
    if (resp.statusCode == 204 || resp.statusCode == 200) return;
    throw _apiError(resp, 'Could not delete the person.');
  }

  /// Lists a person's events including the schedule fields.
  Future<List<Map<String, dynamic>>> listPersonEvents(int personId) async {
    final resp = await dio.get('/api/v1/people/$personId/events');
    if (resp.statusCode == 200 && resp.data is Map) {
      final raw = (resp.data as Map)['events'];
      if (raw is List) {
        return raw.whereType<Map>().map((e) => Map<String, dynamic>.from(e)).toList();
      }
      return const [];
    }
    throw _apiError(resp, 'Could not load the events.');
  }

  /// Creates an event for a person and returns the created record.
  Future<Map<String, dynamic>> createEvent(int personId, Map<String, dynamic> body) async {
    final resp = await dio.post(
      '/api/v1/people/$personId/events',
      data: body,
      options: Options(contentType: 'application/json'),
    );
    if (resp.statusCode == 201 && resp.data is Map) {
      return Map<String, dynamic>.from(resp.data as Map);
    }
    throw _apiError(resp, 'Could not create the event.');
  }

  /// Applies a partial update to an event and returns the updated record.
  Future<Map<String, dynamic>> updateEvent(int id, Map<String, dynamic> body) async {
    final resp = await dio.patch(
      '/api/v1/events/$id',
      data: body,
      options: Options(contentType: 'application/json'),
    );
    if (resp.statusCode == 200 && resp.data is Map) {
      return Map<String, dynamic>.from(resp.data as Map);
    }
    throw _apiError(resp, 'Could not update the event.');
  }

  Future<void> deleteEvent(int id) async {
    final resp = await dio.delete('/api/v1/events/$id');
    if (resp.statusCode == 204 || resp.statusCode == 200) return;
    throw _apiError(resp, 'Could not delete the event.');
  }

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

  Future<Response<dynamic>> createPersonNote(
    int personId, {
    required String note,
    String? noteDate,
  }) =>
      dio.post(
        '/people/$personId/notes',
        data: {
          'note': note,
          if (noteDate != null && noteDate.isNotEmpty) 'note_date': noteDate,
        },
        options: Options(contentType: 'application/json'),
      );

  Future<Response<dynamic>> deletePersonNote(int personId, int noteId) => dio.post(
        '/people/$personId/notes/$noteId/delete',
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
