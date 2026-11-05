import 'package:intl/intl.dart';

String countdownText(int daysRemaining) {
  if (daysRemaining == 0) return 'Today';
  if (daysRemaining == 1) return 'Tomorrow';
  return 'In $daysRemaining days';
}

String countdownShort(int daysRemaining) {
  if (daysRemaining == 0) return 'Today';
  if (daysRemaining == 1) return '1d';
  return '${daysRemaining}d';
}

String? turningAgeLabel(int? turningAge, String type) {
  if (turningAge == null) return null;
  if (type == 'birthday') return 'Turning $turningAge';
  return null;
}

String? yearsTogetherLabel(int? yearsTogether, String type) {
  if (yearsTogether == null) return null;
  if (type == 'anniversary') {
    if (yearsTogether == 1) return '1 year together';
    return '$yearsTogether years together';
  }
  return null;
}

bool isMilestoneAge(int? age) {
  if (age == null) return false;
  return age % 10 == 0 && age > 0;
}

bool isMilestoneYears(int? years) {
  if (years == null) return false;
  return years % 5 == 0 && years > 0;
}

String priceLabel(int? priceCents) {
  if (priceCents == null) return '';
  final dollars = priceCents ~/ 100;
  final cents = priceCents % 100;
  if (cents == 0) return '\$$dollars';
  return '\$$dollars.${cents.toString().padLeft(2, '0')}';
}

String giftStatusLabel(String status) {
  switch (status) {
    case 'idea':
      return 'Idea';
    case 'purchased':
      return 'Purchased';
    case 'given':
      return 'Given';
    case 'archived':
      return 'Archived';
    default:
      return status;
  }
}

String eventTypeLabel(String type) {
  switch (type) {
    case 'birthday':
      return 'Birthday';
    case 'anniversary':
      return 'Anniversary';
    default:
      if (type.isEmpty) return type;
      return type[0].toUpperCase() + type.substring(1);
  }
}

String bucketTitle(String key) {
  switch (key) {
    case 'today':
      return 'Today';
    case 'this_week':
      return 'This week';
    case 'this_month':
      return 'This month';
    case 'later':
      return 'Later';
    default:
      return key;
  }
}

/// Formats a `YYYY-MM-DD` note date for display, falling back to the raw value
/// when it cannot be parsed.
String noteDateLabel(String date) {
  final parsed = DateTime.tryParse(date);
  if (parsed == null) return date;
  return DateFormat('MMM d, y').format(parsed);
}

/// The current local date as `YYYY-MM-DD`.
String isoToday() => isoDate(DateTime.now());

/// A local date as `YYYY-MM-DD`.
String isoDate(DateTime date) => DateFormat('yyyy-MM-dd').format(date);

/// Human-readable byte size, e.g. `820 B`, `1.5 KB`, `2.0 MB`.
String formatDataSize(int bytes) {
  if (bytes < 1024) return '$bytes B';
  const units = ['KB', 'MB', 'GB'];
  var size = bytes / 1024;
  var unit = 0;
  while (size >= 1024 && unit < units.length - 1) {
    size /= 1024;
    unit++;
  }
  return '${size.toStringAsFixed(1)} ${units[unit]}';
}
