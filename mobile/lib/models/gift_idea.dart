class GiftIdea {
  final int id;
  final String title;
  final String notes;
  final int? priceCents;
  final String? url;
  final String status; // idea|purchased|given|archived
  final String createdAt;
  final String updatedAt;

  const GiftIdea({
    required this.id,
    required this.title,
    required this.notes,
    this.priceCents,
    this.url,
    required this.status,
    required this.createdAt,
    required this.updatedAt,
  });

  factory GiftIdea.fromJson(Map<String, dynamic> j) => GiftIdea(
        id: j['id'] as int,
        title: j['title'] as String,
        notes: j['notes'] as String? ?? '',
        priceCents: j['price_cents'] as int?,
        url: j['url'] as String?,
        status: j['status'] as String,
        createdAt: j['created_at'] as String? ?? '',
        updatedAt: j['updated_at'] as String? ?? '',
      );

  Map<String, dynamic> toJson() => {
        'title': title,
        'notes': notes,
        'price_cents': priceCents,
        'url': url,
        'status': status,
      };
}
