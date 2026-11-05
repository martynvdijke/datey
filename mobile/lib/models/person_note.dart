/// A single entry in a person's notes timeline.
class PersonNote {
  final int id;
  final String note;
  final String noteDate;
  final String createdAt;
  final String updatedAt;

  const PersonNote({
    required this.id,
    required this.note,
    required this.noteDate,
    required this.createdAt,
    required this.updatedAt,
  });

  factory PersonNote.fromJson(Map<String, dynamic> j) => PersonNote(
        id: (j['id'] as num?)?.toInt() ?? 0,
        note: j['note'] as String? ?? '',
        noteDate: j['note_date'] as String? ?? '',
        createdAt: j['created_at'] as String? ?? '',
        updatedAt: j['updated_at'] as String? ?? '',
      );
}
