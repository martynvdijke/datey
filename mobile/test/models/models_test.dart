import 'package:flutter_test/flutter_test.dart';
import 'package:datey/models/upcoming_event.dart';
import 'package:datey/models/person_card.dart';
import 'package:datey/models/person_detail.dart';
import 'package:datey/models/gift_idea.dart';
import 'package:datey/models/user.dart';

void main() {
  test('UpcomingEvent fromJson', () {
    final j = {
      'id': 12,
      'person_id': 3,
      'person_name': 'Dana',
      'person_photo_url': '/people/3/photo',
      'group_id': null,
      'group_name': null,
      'type': 'birthday',
      'description': '',
      'date': '2026-10-20',
      'days_remaining': 3,
      'turning_age': 31,
      'years_together': null,
      'calendar_system': 'lunar',
      'lunar_label': '九月十五',
      'is_leap_month': true,
      'is_milestone': true,
      'milestone_label': '30th birthday'
    };
    final e = UpcomingEvent.fromJson(j);
    expect(e.id, 12);
    expect(e.personName, 'Dana');
    expect(e.calendarSystem, 'lunar');
    expect(e.lunarLabel, '九月十五');
    expect(e.isLeapMonth, true);
    expect(e.isMilestone, true);
    expect(e.turningAge, 31);
  });

  test('DashboardData fromJson empty and buckets', () {
    final d = DashboardData.fromJson({'today': [], 'this_week': [], 'this_month': [], 'later': []});
    expect(d.isEmpty, true);
    expect(d.totalCount, 0);
    final d2 = DashboardData.fromJson({
      'today': [
        {
          'id': 1,
          'person_id': 1,
          'person_name': 'A',
          'type': 'birthday',
          'description': '',
          'date': '2026-10-17',
          'days_remaining': 0,
          'calendar_system': 'gregorian',
          'is_leap_month': false,
          'is_milestone': false
        }
      ],
      'this_week': [],
      'this_month': [],
      'later': []
    });
    expect(d2.today.length, 1);
    expect(d2.totalCount, 1);
    expect(d2.isEmpty, false);
  });

  test('PersonCard fromJson', () {
    final j = {
      'id': 3,
      'name': 'Dana',
      'first_name': 'Dana',
      'last_name': '',
      'photo_url': '/people/3/photo',
      'tags': ['family'],
      'groups': [
        {'id': 1, 'name': 'Family'}
      ],
      'next_event': {'type': 'birthday', 'date': '2026-10-20', 'days_remaining': 3}
    };
    final p = PersonCard.fromJson(j);
    expect(p.name, 'Dana');
    expect(p.tags, ['family']);
    expect(p.groups.first.name, 'Family');
    expect(p.nextEvent!.daysRemaining, 3);
  });

  test('PersonDetail fromJson with hidden gifts', () {
    final j = {
      'id': 3,
      'name': 'Dana',
      'first_name': 'Dana',
      'middle_name': '',
      'last_name': '',
      'notes': '',
      'photo_url': '/people/3/photo',
      'tags': [],
      'events': [
        {'id': 5, 'type': 'birthday', 'date': '1995-10-20', 'description': '', 'calendar_system': 'gregorian', 'lunar_leap': false}
      ],
      'gift_ideas': [],
      'gift_ideas_hidden': true,
      'relationships': [
        {'id': 7, 'type': 'partner', 'label': null, 'other_person_id': 4, 'other_person_name': 'Alex', 'direction': 'from'}
      ],
      'groups': [
        {'id': 1, 'name': 'Family'}
      ],
    };
    final d = PersonDetail.fromJson(j);
    expect(d.giftIdeasHidden, true);
    expect(d.giftIdeas, isEmpty);
    expect(d.relationships.first.direction, 'from');
    expect(d.relationships.first.displayLabel(), 'Partner');
    expect(d.events.first.calendarSystem, 'gregorian');
  });

  test('GiftIdea fromJson', () {
    final j = {
      'id': 21,
      'title': 'Book',
      'notes': 'nice',
      'price_cents': 1999,
      'url': 'https://example.com',
      'status': 'idea',
      'created_at': '2026-10-17T10:00:00Z',
      'updated_at': '2026-10-17T10:00:00Z'
    };
    final g = GiftIdea.fromJson(j);
    expect(g.title, 'Book');
    expect(g.priceCents, 1999);
    expect(g.status, 'idea');
  });

  test('AppUser fromJson', () {
    final u = AppUser.fromJson({'id': 1, 'username': 'alice', 'role': 'admin', 'linked_person_id': 3});
    expect(u.linkedPersonId, 3);
    expect(u.username, 'alice');
  });

  test('Group fromJson', () {
    final g = Group.fromJson({'id': 1, 'name': 'Family', 'description': '', 'member_count': 4});
    expect(g.memberCount, 4);
  });

  test('Relationship displayLabel uses type when label null', () {
    final r = Relationship(id: 1, type: 'partner', label: null, otherPersonId: 2, otherPersonName: 'Bob', direction: 'from');
    expect(r.displayLabel(), 'Partner');
    final r2 = Relationship(id: 2, type: 'partner', label: 'My partner', otherPersonId: 2, otherPersonName: 'Bob', direction: 'from');
    expect(r2.displayLabel(), 'My partner');
  });
}
