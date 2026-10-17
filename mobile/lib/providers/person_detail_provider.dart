import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/person_detail.dart';
import '../services/api_client.dart';
import '../services/storage_service.dart';

final personDetailProvider = FutureProvider.family<PersonDetail, int>((ref, id) async {
  final storage = StorageService();
  final baseUrl = await storage.getBaseUrl();
  if (baseUrl == null || baseUrl.isEmpty) throw Exception('No server URL');
  final client = ApiClient(storage: storage, baseUrl: baseUrl);
  final resp = await client.getPerson(id);
  if (resp.statusCode == 200 && resp.data is Map) {
    return PersonDetail.fromJson(resp.data as Map<String, dynamic>);
  }
  throw Exception(resp.data is Map ? resp.data['message'] ?? 'Not found' : 'Failed');
});
