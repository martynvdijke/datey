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
