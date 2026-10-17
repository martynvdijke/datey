import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/gift_idea.dart';
import '../services/api_client.dart';
import '../services/storage_service.dart';
import '../utils/formatters.dart';

class GiftSheet extends ConsumerStatefulWidget {
  final int personId;
  final GiftIdea? existing;
  final VoidCallback onSaved;
  const GiftSheet({super.key, required this.personId, this.existing, required this.onSaved});

  @override
  ConsumerState<GiftSheet> createState() => _GiftSheetState();
}

class _GiftSheetState extends ConsumerState<GiftSheet> {
  final _formKey = GlobalKey<FormState>();
  late TextEditingController _title;
  late TextEditingController _notes;
  late TextEditingController _price;
  late TextEditingController _url;
  String _status = 'idea';
  bool _saving = false;
  String? _error;

  @override
  void initState() {
    super.initState();
    _title = TextEditingController(text: widget.existing?.title ?? '');
    _notes = TextEditingController(text: widget.existing?.notes ?? '');
    _price = TextEditingController(
        text: widget.existing?.priceCents == null ? '' : (widget.existing!.priceCents! / 100).toStringAsFixed(2));
    _url = TextEditingController(text: widget.existing?.url ?? '');
    _status = widget.existing?.status ?? 'idea';
  }

  @override
  void dispose() {
    _title.dispose();
    _notes.dispose();
    _price.dispose();
    _url.dispose();
    super.dispose();
  }

  int? _parsePriceCents(String v) {
    if (v.trim().isEmpty) return null;
    final d = double.tryParse(v.trim());
    if (d == null) return null;
    return (d * 100).round();
  }

  Future<void> _save() async {
    if (!_formKey.currentState!.validate()) return;
    setState(() {
      _saving = true;
      _error = null;
    });
    try {
      final storage = StorageService();
      final baseUrl = await storage.getBaseUrl();
      if (baseUrl == null) throw Exception('No server URL');
      final client = ApiClient(storage: storage, baseUrl: baseUrl);
      final priceCents = _parsePriceCents(_price.text);
      final body = {
        'title': _title.text.trim(),
        'notes': _notes.text.trim(),
        'price_cents': priceCents,
        'url': _url.text.trim().isEmpty ? null : _url.text.trim(),
        'status': _status,
      };
      if (widget.existing == null) {
        final resp = await client.createGift(widget.personId, body);
        if (resp.statusCode == 201 || resp.statusCode == 200) {
          widget.onSaved();
          if (mounted) Navigator.pop(context);
        } else {
          final msg = resp.data is Map ? (resp.data!['message'] as String? ?? 'Failed') : 'Failed';
          setState(() => _error = msg);
        }
      } else {
        final resp = await client.updateGift(widget.personId, widget.existing!.id, body);
        if (resp.statusCode == 200) {
          widget.onSaved();
          if (mounted) Navigator.pop(context);
        } else {
          final msg = resp.data is Map ? (resp.data!['message'] as String? ?? 'Failed to save changes') : 'Failed to save changes';
          setState(() => _error = msg);
        }
      }
    } catch (e) {
      setState(() => _error = e.toString());
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return Padding(
      padding: EdgeInsets.only(bottom: MediaQuery.of(context).viewInsets.bottom),
      child: SingleChildScrollView(
        padding: const EdgeInsets.fromLTRB(20, 12, 20, 24),
        child: Form(
          key: _formKey,
          child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            Center(
              child: Container(width: 36, height: 4, decoration: BoxDecoration(color: scheme.outlineVariant, borderRadius: BorderRadius.circular(2))),
            ),
            const SizedBox(height: 16),
            Text(widget.existing == null ? 'Add gift idea' : 'Edit gift idea',
                style: const TextStyle(fontSize: 20, fontWeight: FontWeight.w800)),
            const SizedBox(height: 4),
            Text('Save something thoughtful for later.',
                style: TextStyle(color: scheme.onSurfaceVariant)),
            const SizedBox(height: 20),
            TextFormField(
              controller: _title,
              decoration: const InputDecoration(labelText: 'Title *', border: OutlineInputBorder(), hintText: 'e.g. Cookbook'),
              validator: (v) => v == null || v.trim().isEmpty ? 'Title is required' : null,
              textCapitalization: TextCapitalization.sentences,
            ),
            const SizedBox(height: 12),
            TextFormField(
              controller: _notes,
              decoration: const InputDecoration(labelText: 'Notes', border: OutlineInputBorder(), hintText: 'Size, color, why they’d love it'),
              maxLines: 3,
            ),
            const SizedBox(height: 12),
            Row(children: [
              Expanded(
                child: TextFormField(
                  controller: _price,
                  decoration: const InputDecoration(labelText: 'Price', border: OutlineInputBorder(), prefixText: '\$ ', hintText: '24.99'),
                  keyboardType: const TextInputType.numberWithOptions(decimal: true),
                  validator: (v) {
                    if (v == null || v.trim().isEmpty) return null;
                    if (double.tryParse(v.trim()) == null) return 'Invalid price';
                    return null;
                  },
                ),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: DropdownButtonFormField<String>(
                  initialValue: _status,
                  decoration: const InputDecoration(labelText: 'Status', border: OutlineInputBorder()),
                  items: const [
                    DropdownMenuItem(value: 'idea', child: Text('Idea')),
                    DropdownMenuItem(value: 'purchased', child: Text('Purchased')),
                    DropdownMenuItem(value: 'given', child: Text('Given')),
                    DropdownMenuItem(value: 'archived', child: Text('Archived')),
                  ],
                  onChanged: (v) => setState(() => _status = v ?? 'idea'),
                ),
              ),
            ]),
            const SizedBox(height: 12),
            TextFormField(
              controller: _url,
              decoration: const InputDecoration(labelText: 'Link', border: OutlineInputBorder(), hintText: 'https://example.com'),
              keyboardType: TextInputType.url,
              validator: (v) {
                if (v == null || v.trim().isEmpty) return null;
                final uri = Uri.tryParse(v.trim());
                if (uri == null || !uri.hasScheme) return 'Invalid URL';
                return null;
              },
            ),
            if (_error != null) ...[
              const SizedBox(height: 12),
              Container(
                padding: const EdgeInsets.all(12),
                decoration: BoxDecoration(color: scheme.errorContainer, borderRadius: BorderRadius.circular(12)),
                child: Row(children: [
                  Icon(Icons.error_outline, size: 18, color: scheme.onErrorContainer),
                  const SizedBox(width: 8),
                  Expanded(child: Text(_error!, style: TextStyle(color: scheme.onErrorContainer, fontSize: 13))),
                ]),
              ),
            ],
            const SizedBox(height: 20),
            SizedBox(
              width: double.infinity,
              child: FilledButton(
                onPressed: _saving ? null : _save,
                child: _saving
                    ? const SizedBox(height: 20, width: 20, child: CircularProgressIndicator(strokeWidth: 2, color: Colors.white))
                    : Text(widget.existing == null ? 'Save gift idea' : 'Save changes'),
              ),
            ),
          ]),
        ),
      ),
    );
  }
}

class GiftStatusChip extends StatelessWidget {
  final String status;
  const GiftStatusChip({super.key, required this.status});
  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    Color bg;
    Color fg;
    IconData icon;
    switch (status) {
      case 'purchased':
        bg = scheme.primaryContainer;
        fg = scheme.onPrimaryContainer;
        icon = Icons.shopping_bag;
        break;
      case 'given':
        bg = const Color(0xFFE6F4EA);
        fg = const Color(0xFF1B5E20);
        icon = Icons.card_giftcard;
        break;
      case 'archived':
        bg = scheme.surfaceContainerHighest;
        fg = scheme.onSurfaceVariant;
        icon = Icons.archive;
        break;
      default:
        bg = scheme.secondaryContainer;
        fg = scheme.onSecondaryContainer;
        icon = Icons.lightbulb;
    }
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
      decoration: BoxDecoration(color: bg, borderRadius: BorderRadius.circular(20)),
      child: Row(mainAxisSize: MainAxisSize.min, children: [
        Icon(icon, size: 12, color: fg),
        const SizedBox(width: 4),
        Text(giftStatusLabel(status), style: TextStyle(fontSize: 11, fontWeight: FontWeight.w700, color: fg)),
      ]),
    );
  }
}
