import 'package:flutter/material.dart';
import '../services/api_client.dart';
import '../services/storage_service.dart';
import '../utils/formatters.dart';

class NoteSheet extends StatefulWidget {
  final int personId;
  final VoidCallback onSaved;
  const NoteSheet({super.key, required this.personId, required this.onSaved});

  @override
  State<NoteSheet> createState() => _NoteSheetState();
}

class _NoteSheetState extends State<NoteSheet> {
  final _formKey = GlobalKey<FormState>();
  final _note = TextEditingController();
  final _date = TextEditingController(text: isoToday());
  bool _saving = false;
  String? _error;

  @override
  void dispose() {
    _note.dispose();
    _date.dispose();
    super.dispose();
  }

  Future<void> _pickDate() async {
    final now = DateTime.now();
    final initial = DateTime.tryParse(_date.text) ?? now;
    final picked = await showDatePicker(
      context: context,
      initialDate: initial,
      firstDate: DateTime(1900),
      lastDate: DateTime(now.year + 10, 12, 31),
    );
    if (picked != null) {
      setState(() => _date.text = isoDate(picked));
    }
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
      final resp = await client.createPersonNote(
        widget.personId,
        note: _note.text.trim(),
        noteDate: _date.text.trim().isEmpty ? null : _date.text.trim(),
      );
      if (resp.statusCode == 201 || resp.statusCode == 200) {
        widget.onSaved();
        if (mounted) Navigator.pop(context);
      } else {
        final msg = resp.data is Map ? (resp.data!['message'] as String? ?? 'Failed to save note') : 'Failed to save note';
        setState(() => _error = msg);
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
            const Text('Add note', style: TextStyle(fontSize: 20, fontWeight: FontWeight.w800)),
            const SizedBox(height: 4),
            Text('Capture something worth remembering.', style: TextStyle(color: scheme.onSurfaceVariant)),
            const SizedBox(height: 20),
            TextFormField(
              controller: _note,
              decoration: const InputDecoration(
                labelText: 'Note *',
                border: OutlineInputBorder(),
                hintText: 'e.g. Loves hiking on weekends',
                alignLabelWithHint: true,
              ),
              maxLines: 4,
              minLines: 3,
              textCapitalization: TextCapitalization.sentences,
              validator: (v) => v == null || v.trim().isEmpty ? 'Note is required' : null,
            ),
            const SizedBox(height: 12),
            TextFormField(
              controller: _date,
              readOnly: true,
              onTap: _pickDate,
              decoration: const InputDecoration(
                labelText: 'Date',
                border: OutlineInputBorder(),
                suffixIcon: Icon(Icons.calendar_today_outlined),
              ),
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
                    : const Text('Save note'),
              ),
            ),
          ]),
        ),
      ),
    );
  }
}
