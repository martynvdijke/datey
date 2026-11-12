import 'package:flutter_test/flutter_test.dart';
import 'package:datey/utils/api_errors.dart';

void main() {
  group('apiCodeMessage', () {
    test('maps known codes', () {
      expect(apiCodeMessage('required'), 'This field is required');
      expect(apiCodeMessage('date_invalid'), 'Enter a valid date (YYYY-MM-DD)');
      expect(apiCodeMessage('end_date_before_start'), 'End date cannot be before the start date');
      expect(apiCodeMessage('frequency_non_annual'),
          'Only annual repetition is supported for non-Gregorian calendars');
    });

    test('falls back for unknown codes', () {
      expect(apiCodeMessage('something_new'), 'Invalid value');
    });
  });

  group('apiFieldMessages', () {
    test('converts every field code', () {
      final mapped = apiFieldMessages({'name': 'required', 'lunar_day': 'day_invalid'});
      expect(mapped, {
        'name': 'This field is required',
        'lunar_day': 'Invalid day',
      });
    });
  });
}
