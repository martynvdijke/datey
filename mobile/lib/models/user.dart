class AppUser {
  final int id;
  final String username;
  final String role;
  final int? linkedPersonId;

  const AppUser({
    required this.id,
    required this.username,
    required this.role,
    this.linkedPersonId,
  });

  factory AppUser.fromJson(Map<String, dynamic> j) => AppUser(
        id: j['id'] as int,
        username: j['username'] as String,
        role: j['role'] as String,
        linkedPersonId: j['linked_person_id'] as int?,
      );
}
