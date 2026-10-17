import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/upcoming_event.dart';
import '../services/api_client.dart';
import '../services/storage_service.dart';

final dashboardProvider = FutureProvider<DashboardData>((ref) async {
  final storage = StorageService();
  final baseUrl = await storage.getBaseUrl();
  if (baseUrl == null || baseUrl.isEmpty) throw Exception('No server URL');
  final client = ApiClient(storage: storage, baseUrl: baseUrl);
  final resp = await client.getDashboard();
  if (resp.statusCode == 200 && resp.data is Map) {
    return DashboardData.fromJson(resp.data as Map<String, dynamic>);
  }
  throw Exception(resp.data is Map ? resp.data['message'] ?? 'Failed to load dashboard' : 'Failed');
});
