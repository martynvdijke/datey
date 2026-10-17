class UpcomingEvent {
  final int id;
  final int personId;
  final String personName;
  final String? personPhotoUrl;
  final int? groupId;
  final String? groupName;
  final String type; // birthday, anniversary, etc
  final String description;
  final String date; // YYYY-MM-DD
  final int daysRemaining;
  final int? turningAge;
  final int? yearsTogether;
  final String calendarSystem; // gregorian|lunar
  final String? lunarLabel;
  final bool isLeapMonth;
  final bool isMilestone;
  final String? milestoneLabel;

  const UpcomingEvent({
    required this.id,
    required this.personId,
    required this.personName,
    this.personPhotoUrl,
    this.groupId,
    this.groupName,
    required this.type,
    required this.description,
    required this.date,
    required this.daysRemaining,
    this.turningAge,
    this.yearsTogether,
    required this.calendarSystem,
    this.lunarLabel,
    required this.isLeapMonth,
    required this.isMilestone,
    this.milestoneLabel,
  });

  factory UpcomingEvent.fromJson(Map<String, dynamic> j) => UpcomingEvent(
        id: j['id'] as int,
        personId: j['person_id'] as int,
        personName: j['person_name'] as String,
        personPhotoUrl: j['person_photo_url'] as String?,
        groupId: j['group_id'] as int?,
        groupName: j['group_name'] as String?,
        type: j['type'] as String,
        description: j['description'] as String? ?? '',
        date: j['date'] as String,
        daysRemaining: j['days_remaining'] as int,
        turningAge: j['turning_age'] as int?,
        yearsTogether: j['years_together'] as int?,
        calendarSystem: j['calendar_system'] as String? ?? 'gregorian',
        lunarLabel: j['lunar_label'] as String?,
        isLeapMonth: j['is_leap_month'] as bool? ?? false,
        isMilestone: j['is_milestone'] as bool? ?? false,
        milestoneLabel: j['milestone_label'] as String?,
      );
}

class DashboardData {
  final List<UpcomingEvent> today;
  final List<UpcomingEvent> thisWeek;
  final List<UpcomingEvent> thisMonth;
  final List<UpcomingEvent> later;

  const DashboardData({
    required this.today,
    required this.thisWeek,
    required this.thisMonth,
    required this.later,
  });

  factory DashboardData.fromJson(Map<String, dynamic> j) => DashboardData(
        today: (j['today'] as List<dynamic>? ?? [])
            .map((e) => UpcomingEvent.fromJson(e as Map<String, dynamic>))
            .toList(),
        thisWeek: (j['this_week'] as List<dynamic>? ?? [])
            .map((e) => UpcomingEvent.fromJson(e as Map<String, dynamic>))
            .toList(),
        thisMonth: (j['this_month'] as List<dynamic>? ?? [])
            .map((e) => UpcomingEvent.fromJson(e as Map<String, dynamic>))
            .toList(),
        later: (j['later'] as List<dynamic>? ?? [])
            .map((e) => UpcomingEvent.fromJson(e as Map<String, dynamic>))
            .toList(),
      );

  bool get isEmpty =>
      today.isEmpty && thisWeek.isEmpty && thisMonth.isEmpty && later.isEmpty;

  int get totalCount =>
      today.length + thisWeek.length + thisMonth.length + later.length;
}
