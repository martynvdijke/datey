class PersonCard {
  final int id;
  final String name;
  final String firstName;
  final String lastName;
  final String? photoUrl;
  final List<String> tags;
  final List<GroupRef> groups;
  final NextEventRef? nextEvent;

  const PersonCard({
    required this.id,
    required this.name,
    required this.firstName,
    required this.lastName,
    this.photoUrl,
    required this.tags,
    required this.groups,
    this.nextEvent,
  });

  factory PersonCard.fromJson(Map<String, dynamic> j) => PersonCard(
        id: j['id'] as int,
        name: j['name'] as String,
        firstName: j['first_name'] as String? ?? '',
        lastName: j['last_name'] as String? ?? '',
        photoUrl: j['photo_url'] as String?,
        tags: (j['tags'] as List<dynamic>? ?? []).cast<String>(),
        groups: (j['groups'] as List<dynamic>? ?? [])
            .map((e) => GroupRef.fromJson(e as Map<String, dynamic>))
            .toList(),
        nextEvent: j['next_event'] == null
            ? null
            : NextEventRef.fromJson(j['next_event'] as Map<String, dynamic>),
      );
}

class GroupRef {
  final int id;
  final String name;
  const GroupRef({required this.id, required this.name});
  factory GroupRef.fromJson(Map<String, dynamic> j) =>
      GroupRef(id: j['id'] as int, name: j['name'] as String);
}

class NextEventRef {
  final String type;
  final String date;
  final int daysRemaining;
  const NextEventRef({
    required this.type,
    required this.date,
    required this.daysRemaining,
  });
  factory NextEventRef.fromJson(Map<String, dynamic> j) => NextEventRef(
        type: j['type'] as String,
        date: j['date'] as String,
        daysRemaining: j['days_remaining'] as int,
      );
}

class Group {
  final int id;
  final String name;
  final String description;
  final int memberCount;
  const Group({
    required this.id,
    required this.name,
    required this.description,
    required this.memberCount,
  });
  factory Group.fromJson(Map<String, dynamic> j) => Group(
        id: j['id'] as int,
        name: j['name'] as String,
        description: j['description'] as String? ?? '',
        memberCount: j['member_count'] as int? ?? 0,
      );
}
