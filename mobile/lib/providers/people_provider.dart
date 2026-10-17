import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/person_card.dart';
import '../services/api_client.dart';
import '../services/storage_service.dart';

class PeopleQuery {
  final String q;
  final int? group;
  const PeopleQuery({this.q = '', this.group});
  @override
  bool operator ==(Object other) =>
      other is PeopleQuery && other.q == q && other.group == group;
  @override
  int get hashCode => Object.hash(q, group);
}

class PeopleState {
  final List<PersonCard> people;
  final List<Group> groups;
  final int total;
  const PeopleState({required this.people, required this.groups, required this.total});
}

final peopleQueryProvider = StateProvider<PeopleQuery>((ref) => const PeopleQuery());

final peopleProvider = FutureProvider<PeopleState>((ref) async {
  final q = ref.watch(peopleQueryProvider);
  final storage = StorageService();
  final baseUrl = await storage.getBaseUrl();
  if (baseUrl == null || baseUrl.isEmpty) throw Exception('No server URL');
  final client = ApiClient(storage: storage, baseUrl: baseUrl);
  final resp = await client.getPeople(q: q.q, group: q.group);
  if (resp.statusCode == 200 && resp.data is Map) {
    final m = resp.data as Map<String, dynamic>;
    final people = (m['people'] as List<dynamic>? ?? [])
        .map((e) => PersonCard.fromJson(e as Map<String, dynamic>))
        .toList();
    final groups = (m['groups'] as List<dynamic>? ?? [])
        .map((e) => Group.fromJson(e as Map<String, dynamic>))
        .toList();
    final total = m['total'] as int? ?? people.length;
    return PeopleState(people: people, groups: groups, total: total);
  }
  throw Exception('Failed to load people');
});

final groupsProvider = FutureProvider<List<Group>>((ref) async {
  final storage = StorageService();
  final baseUrl = await storage.getBaseUrl();
  if (baseUrl == null || baseUrl.isEmpty) throw Exception('No server URL');
  final client = ApiClient(storage: storage, baseUrl: baseUrl);
  final resp = await client.getGroups();
  if (resp.statusCode == 200 && resp.data is Map) {
    return (resp.data['groups'] as List<dynamic>? ?? [])
        .map((e) => Group.fromJson(e as Map<String, dynamic>))
        .toList();
  }
  throw Exception('Failed to load groups');
});
