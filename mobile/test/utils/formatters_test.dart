import 'package:flutter_test/flutter_test.dart';
import 'package:datey/utils/formatters.dart';

void main() {
  group('countdownText', () {
    test('today', () => expect(countdownText(0), 'Today'));
    test('tomorrow', () => expect(countdownText(1), 'Tomorrow'));
    test('multiple', () => expect(countdownText(5), 'In 5 days'));
    test('short today', () => expect(countdownShort(0), 'Today'));
    test('short 1d', () => expect(countdownShort(1), '1d'));
    test('short 10d', () => expect(countdownShort(10), '10d'));
  });

  group('age / years together', () {
    test('turning age birthday', () => expect(turningAgeLabel(30, 'birthday'), 'Turning 30'));
    test('turning age null', () => expect(turningAgeLabel(null, 'birthday'), isNull));
    test('turning age anniversary returns null', () => expect(turningAgeLabel(5, 'anniversary'), isNull));
    test('years together', () => expect(yearsTogetherLabel(10, 'anniversary'), '10 years together'));
    test('1 year together', () => expect(yearsTogetherLabel(1, 'anniversary'), '1 year together'));
    test('years together birthday null', () => expect(yearsTogetherLabel(3, 'birthday'), isNull));
  });

  group('milestone', () {
    test('milestone age true', () => expect(isMilestoneAge(30), true));
    test('milestone age false', () => expect(isMilestoneAge(31), false));
    test('milestone age zero false', () => expect(isMilestoneAge(0), false));
    test('milestone years 5 true', () => expect(isMilestoneYears(5), true));
    test('milestone years 10 true', () => expect(isMilestoneYears(10), true));
    test('milestone years 6 false', () => expect(isMilestoneYears(6), false));
  });

  group('priceLabel', () {
    test('null', () => expect(priceLabel(null), ''));
    test('1999 cents', () => expect(priceLabel(1999), r'$19.99'));
    test('2000 cents', () => expect(priceLabel(2000), r'$20'));
    test('0 cents', () => expect(priceLabel(0), r'$0'));
    test('5 cents', () => expect(priceLabel(5), r'$0.05'));
  });

  group('labels', () {
    test('gift status', () {
      expect(giftStatusLabel('idea'), 'Idea');
      expect(giftStatusLabel('purchased'), 'Purchased');
      expect(giftStatusLabel('given'), 'Given');
      expect(giftStatusLabel('archived'), 'Archived');
    });
    test('event type', () {
      expect(eventTypeLabel('birthday'), 'Birthday');
      expect(eventTypeLabel('anniversary'), 'Anniversary');
      expect(eventTypeLabel('custom'), 'Custom');
    });
    test('bucket titles', () {
      expect(bucketTitle('today'), 'Today');
      expect(bucketTitle('this_week'), 'This week');
      expect(bucketTitle('this_month'), 'This month');
      expect(bucketTitle('later'), 'Later');
    });
  });
}
