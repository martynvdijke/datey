import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:dio/dio.dart';
import '../models/user.dart';
import '../services/api_client.dart';
import '../services/storage_service.dart';

final storageServiceProvider = Provider<StorageService>((ref) => StorageService());

final apiClientProvider = Provider<ApiClient>((ref) {
  final storage = ref.watch(storageServiceProvider);
  // We will override baseUrl dynamically via async provider below.
  // Default dio with empty baseUrl; actual baseUrl injected after onboarding.
  return ApiClient(storage: storage);
});

// Holds current baseUrl (async)
final baseUrlProvider = FutureProvider<String?>((ref) async {
  final s = ref.watch(storageServiceProvider);
  return s.getBaseUrl();
});

// Dio with baseUrl resolved
final dioProvider = FutureProvider<Dio>((ref) async {
  final storage = ref.watch(storageServiceProvider);
  final baseUrl = await storage.getBaseUrl();
  final client = ApiClient(storage: storage, baseUrl: baseUrl ?? '');
  return client.dio;
});

final authStateProvider = StateNotifierProvider<AuthNotifier, AsyncValue<AppUser?>>((ref) {
  return AuthNotifier(ref);
});

class AuthNotifier extends StateNotifier<AsyncValue<AppUser?>> {
  final Ref ref;
  AuthNotifier(this.ref) : super(const AsyncValue.loading()) {
    _init();
  }

  Future<void> _init() async {
    final storage = ref.read(storageServiceProvider);
    final baseUrl = await storage.getBaseUrl();
    if (baseUrl == null || baseUrl.isEmpty) {
      state = const AsyncValue.data(null);
      return;
    }
    await validateSession();
  }

  Future<bool> validateSession() async {
    state = const AsyncValue.loading();
    try {
      final storage = ref.read(storageServiceProvider);
      final baseUrl = await storage.getBaseUrl();
      if (baseUrl == null || baseUrl.isEmpty) {
        state = const AsyncValue.data(null);
        return false;
      }
      final client = ApiClient(storage: storage, baseUrl: baseUrl);
      final resp = await client.getMe();
      if (resp.statusCode == 200 && resp.data is Map) {
        final user = AppUser.fromJson(resp.data as Map<String, dynamic>);
        state = AsyncValue.data(user);
        return true;
      } else if (resp.statusCode == 401) {
        await storage.clearAuth();
        state = const AsyncValue.data(null);
        return false;
      } else {
        state = AsyncValue.error('Session check failed: ${resp.statusCode}', StackTrace.current);
        return false;
      }
    } catch (e, st) {
      state = AsyncValue.error(e, st);
      return false;
    }
  }

  Future<bool> login(String baseUrl, String username, String password) async {
    state = const AsyncValue.loading();
    try {
      final storage = ref.read(storageServiceProvider);
      await storage.setBaseUrl(baseUrl);
      final client = ApiClient(storage: storage, baseUrl: baseUrl);
      final resp = await client.login(username, password);
      if (resp.statusCode == 200 && resp.data is Map) {
        // cookies already persisted via interceptor
        final user = AppUser.fromJson(resp.data as Map<String, dynamic>);
        state = AsyncValue.data(user);
        ref.invalidate(baseUrlProvider);
        return true;
      } else {
        state = AsyncValue.error(resp.data['message'] ?? 'Login failed', StackTrace.current);
        return false;
      }
    } catch (e, st) {
      state = AsyncValue.error(e, st);
      return false;
    }
  }

  Future<void> logout() async {
    final storage = ref.read(storageServiceProvider);
    await storage.clearAuth();
    state = const AsyncValue.data(null);
  }

  Future<void> setBaseUrl(String url) async {
    final storage = ref.read(storageServiceProvider);
    await storage.setBaseUrl(url);
    ref.invalidate(baseUrlProvider);
  }

  // After OIDC flow, validate
  Future<bool> afterOidc(String baseUrl) async {
    return validateSession();
  }
}

final oidcEnabledProvider = FutureProvider<bool>((ref) async {
  final storage = ref.read(storageServiceProvider);
  final baseUrl = await storage.getBaseUrl();
  if (baseUrl == null || baseUrl.isEmpty) return false;
  try {
    final client = ApiClient(storage: storage, baseUrl: baseUrl);
    final resp = await client.getOidcStatus();
    if (resp.statusCode == 200 && resp.data is Map) {
      return resp.data['enabled'] as bool? ?? false;
    }
    return false;
  } catch (_) {
    return false;
  }
});
