import 'gift_idea.dart';
import 'person_card.dart';

class PersonDetail {
  final int id;
  final String name;
  final String firstName;
  final String middleName;
  final String lastName;
  final String notes;
  final String? photoUrl;
  final String? immichPersonId;
  final bool notifyBirthdays;
  final String timezone;
  final List<String> tags;
  final List<PersonEvent> events;
  final List<GiftIdea> giftIdeas;
  final bool giftIdeasHidden;
  final List<Relationship> relationships;
  final List<GroupRef> groups;

  const PersonDetail({
    required this.id,
    required this.name,
    required this.firstName,
    required this.middleName,
    required this.lastName,
    required this.notes,
    this.photoUrl,
    this.immichPersonId,
    required this.notifyBirthdays,
    required this.timezone,
    required this.tags,
    required this.events,
    required this.giftIdeas,
    required this.giftIdeasHidden,
    required this.relationships,
    required this.groups,
  });

  factory PersonDetail.fromJson(Map<String, dynamic> j) => PersonDetail(
        id: j['id'] as int,
        name: j['name'] as String,
        firstName: j['first_name'] as String? ?? '',
        middleName: j['middle_name'] as String? ?? '',
        lastName: j['last_name'] as String? ?? '',
        notes: j['notes'] as String? ?? '',
        photoUrl: j['photo_url'] as String?,
        immichPersonId: j['immich_person_id'] as String?,
        notifyBirthdays: j['notify_birthdays'] as bool? ?? true,
        timezone: j['timezone'] as String? ?? 'UTC',
        tags: (j['tags'] as List<dynamic>? ?? []).cast<String>(),
        events: (j['events'] as List<dynamic>? ?? [])
            .map((e) => PersonEvent.fromJson(e as Map<String, dynamic>))
            .toList(),
        giftIdeas: (j['gift_ideas'] as List<dynamic>? ?? [])
            .map((e) => GiftIdea.fromJson(e as Map<String, dynamic>))
            .toList(),
        giftIdeasHidden: j['gift_ideas_hidden'] as bool? ?? false,
        relationships: (j['relationships'] as List<dynamic>? ?? [])
            .map((e) => Relationship.fromJson(e as Map<String, dynamic>))
            .toList(),
        groups: (j['groups'] as List<dynamic>? ?? [])
            .map((e) => GroupRef.fromJson(e as Map<String, dynamic>))
            .toList(),
      );
}

class PersonEvent {
  final int id;
  final String type;
  final String date;
  final String description;
  final String calendarSystem;
  final int? lunarMonth;
  final int? lunarDay;
  final bool lunarLeap;

  const PersonEvent({
    required this.id,
    required this.type,
    required this.date,
    required this.description,
    required this.calendarSystem,
    this.lunarMonth,
    this.lunarDay,
    required this.lunarLeap,
  });

  factory PersonEvent.fromJson(Map<String, dynamic> j) => PersonEvent(
        id: j['id'] as int,
        type: j['type'] as String,
        date: j['date'] as String,
        description: j['description'] as String? ?? '',
        calendarSystem: j['calendar_system'] as String? ?? 'gregorian',
        lunarMonth: j['lunar_month'] as int?,
        lunarDay: j['lunar_day'] as int?,
        lunarLeap: j['lunar_leap'] as bool? ?? false,
      );
}

class Relationship {
  final int id;
  final String type;
  final String? label;
  final int otherPersonId;
  final String otherPersonName;
  final String direction; // from|to

  const Relationship({
    required this.id,
    required this.type,
    this.label,
    required this.otherPersonId,
    required this.otherPersonName,
    required this.direction,
  });

  factory Relationship.fromJson(Map<String, dynamic> j) => Relationship(
        id: j['id'] as int,
        type: j['type'] as String,
        label: j['label'] as String?,
        otherPersonId: j['other_person_id'] as int,
        otherPersonName: j['other_person_name'] as String,
        direction: j['direction'] as String,
      );

  String displayLabel() {
    if (label != null && label!.isNotEmpty) return label!;
    // Capitalize type
    return type[0].toUpperCase() + type.substring(1);
  }
}
