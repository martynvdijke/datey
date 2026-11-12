/// Maps the machine-readable field codes returned by the versioned JSON write
/// API onto short, user-facing messages.
///
/// The server never sends localized strings, so the app owns the wording here.
/// Keys mirror `internal/web/events.go` and `internal/web/api_v1_write.go`.
library;

const Map<String, String> _codeMessages = {
  'required': 'This field is required',
  'invalid_group': 'One of the selected groups no longer exists',
  'unknown_group': 'One of the selected groups no longer exists',
  'invalid_tag': 'Tags may only contain lowercase letters, digits, "-" and "_"',
  'type_required': 'Event type is required',
  'date_required': 'Date is required',
  'date_invalid': 'Enter a valid date (YYYY-MM-DD)',
  'frequency_invalid': 'Unknown repeat frequency',
  'frequency_non_annual':
      'Only annual repetition is supported for non-Gregorian calendars',
  'interval_invalid': 'Interval must be between 1 and 120',
  'count_invalid': 'Number of occurrences must be at least 1',
  'end_date_invalid': 'Enter a valid end date',
  'end_date_before_start': 'End date cannot be before the start date',
  'month_required': 'Month is required',
  'month_invalid': 'Invalid month',
  'month_invalid_hebrew': 'Invalid Hebrew month',
  'day_required': 'Day is required',
  'day_invalid': 'Invalid day',
  'day_invalid_hebrew': 'Invalid day for that Hebrew month',
  'day_invalid_hijri': 'Invalid day for that Hijri month',
};

/// Human text for one server field code.
String apiCodeMessage(String code) => _codeMessages[code] ?? 'Invalid value';

/// Converts the server `fields` payload into field -> message.
Map<String, String> apiFieldMessages(Map<String, String> fields) =>
    fields.map((field, code) => MapEntry(field, apiCodeMessage(code)));
