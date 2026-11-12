import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/person_detail.dart';
import '../providers/people_provider.dart';
import '../services/api_client.dart';
import '../services/storage_service.dart';
import '../utils/api_errors.dart';

/// Create/edit form for a person, shown as a modal bottom sheet.
///
/// Mirrors the web person form: structured first/middle/last name, notes,
/// group membership, tags and the per-person birthday reminder opt-out.
class PersonFormSheet extends ConsumerStatefulWidget {
  final PersonDetail? existing;
  final VoidCallback onSaved;

  const PersonFormSheet({super.key, this.existing, required this.onSaved});

  @override
  ConsumerState<PersonFormSheet> createState() => _PersonFormSheetState();
}

class _PersonFormSheetState extends ConsumerState<PersonFormSheet> {
  final _formKey = GlobalKey<FormState>();
  late TextEditingController _first;
  late TextEditingController _middle;
  late TextEditingController _last;
  late TextEditingController _notes;
  late TextEditingController _tags;
  final Set<int> _groupIds = {};
  bool _notifyBirthdays = true;
  bool _saving = false;
  String? _error;
  Map<String, String> _fieldErrors = {};

  bool get _isEdit => widget.existing != null;

  @override
  void initState() {
    super.initState();
    final p = widget.existing;
    var first = p?.firstName ?? '';
    var middle = p?.middleName ?? '';
    var last = p?.lastName ?? '';
    // Legacy rows only have a display name; split it so editing does not
    // silently blank the name (same heuristic as the web edit form).
    if (p != null && first.isEmpty && middle.isEmpty && last.isEmpty) {
      final parts = p.name.trim().split(RegExp(r'\s+')).where((s) => s.isNotEmpty).toList();
      if (parts.isNotEmpty) {
        first = parts.first;
        last = parts.length > 1 ? parts.sublist(1).join(' ') : '';
      }
    }
    _first = TextEditingController(text: first);
    _middle = TextEditingController(text: middle);
    _last = TextEditingController(text: last);
    _notes = TextEditingController(text: p?.notes ?? '');
    _tags = TextEditingController(text: p?.tags.join(', ') ?? '');
    _notifyBirthdays = p?.notifyBirthdays ?? true;
    if (p != null) {
      _groupIds.addAll(p.groups.map((g) => g.id));
    }
  }

  @override
  void dispose() {
    _first.dispose();
    _middle.dispose();
    _last.dispose();
    _notes.dispose();
    _tags.dispose();
    super.dispose();
  }

  /// Error text for a field. The API reports a missing display name as `name`;
  /// the form has no separate name field, so it lands on the first-name row.
  String? _err(String field) {
    if (_fieldErrors.containsKey(field)) return _fieldErrors[field];
    if (field == 'first_name') return _fieldErrors['name'];
    return null;
  }

  List<String> _tagList() => _tags.text
      .split(',')
      .map((t) => t.trim())
      .where((t) => t.isNotEmpty)
      .toList();

  Future<void> _save() async {
    setState(() {
      _fieldErrors = {};
      _error = null;
    });
    final first = _first.text.trim();
    final middle = _middle.text.trim();
    final last = _last.text.trim();
    if (!_formKey.currentState!.validate()) return;
    if (first.isEmpty && middle.isEmpty && last.isEmpty) {
      setState(() => _fieldErrors = {'first_name': 'Enter a name'});
      return;
    }

    setState(() => _saving = true);
    try {
      final storage = StorageService();
      final baseUrl = await storage.getBaseUrl();
      if (baseUrl == null) throw Exception('No server URL');
      final client = ApiClient(storage: storage, baseUrl: baseUrl);
      final body = <String, dynamic>{
        'first_name': first,
        'middle_name': middle,
        'last_name': last,
        'notes': _notes.text.trim(),
        'groups': _groupIds.toList(),
        'tags': _tagList(),
        'notify_birthdays': _notifyBirthdays,
      };
      if (_isEdit) {
        await client.updatePerson(widget.existing!.id, body);
      } else {
        await client.createPerson(body);
      }
      widget.onSaved();
      if (mounted) Navigator.pop(context);
    } on ApiValidationException catch (e) {
      if (mounted) setState(() => _fieldErrors = apiFieldMessages(e.fields));
    } on ApiException catch (e) {
      if (mounted) setState(() => _error = e.message);
    } catch (_) {
      // Dio exceptions (5xx/network) carry no user-safe detail.
      if (mounted) setState(() => _error = 'Could not reach the server. Please try again.');
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    final groupsAsync = ref.watch(groupsProvider);

    return Padding(
      padding: EdgeInsets.only(bottom: MediaQuery.of(context).viewInsets.bottom),
      child: SingleChildScrollView(
        padding: const EdgeInsets.fromLTRB(20, 12, 20, 24),
        child: Form(
          key: _formKey,
          child: Column(crossAxisAlignment: CrossAxisAlignment.start, children: [
            Center(
              child: Container(
                width: 36,
                height: 4,
                decoration: BoxDecoration(color: scheme.outlineVariant, borderRadius: BorderRadius.circular(2)),
              ),
            ),
            const SizedBox(height: 16),
            Text(_isEdit ? 'Edit person' : 'Add person',
                style: const TextStyle(fontSize: 20, fontWeight: FontWeight.w800)),
            const SizedBox(height: 4),
            Text(_isEdit ? 'Update the details you keep about this person.' : 'Save someone worth remembering.',
                style: TextStyle(color: scheme.onSurfaceVariant)),
            const SizedBox(height: 20),
            TextFormField(
              controller: _first,
              textCapitalization: TextCapitalization.words,
              decoration: InputDecoration(
                labelText: 'First name *',
                border: const OutlineInputBorder(),
                errorText: _err('first_name'),
              ),
            ),
            const SizedBox(height: 12),
            Row(children: [
              Expanded(
                child: TextFormField(
                  controller: _middle,
                  textCapitalization: TextCapitalization.words,
                  decoration: InputDecoration(
                    labelText: 'Middle name',
                    border: const OutlineInputBorder(),
                    errorText: _err('middle_name'),
                  ),
                ),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: TextFormField(
                  controller: _last,
                  textCapitalization: TextCapitalization.words,
                  decoration: InputDecoration(
                    labelText: 'Last name',
                    border: const OutlineInputBorder(),
                    errorText: _err('last_name'),
                  ),
                ),
              ),
            ]),
            const SizedBox(height: 12),
            TextFormField(
              controller: _notes,
              maxLines: 3,
              minLines: 2,
              textCapitalization: TextCapitalization.sentences,
              decoration: InputDecoration(
                labelText: 'Notes',
                border: const OutlineInputBorder(),
                alignLabelWithHint: true,
                errorText: _err('notes'),
              ),
            ),
            const SizedBox(height: 12),
            TextFormField(
              controller: _tags,
              decoration: InputDecoration(
                labelText: 'Tags',
                hintText: 'friend, family (comma separated)',
                border: const OutlineInputBorder(),
                errorText: _err('tags'),
              ),
            ),
            const SizedBox(height: 20),
            Text('Groups', style: TextStyle(fontWeight: FontWeight.w700, color: scheme.onSurface)),
            const SizedBox(height: 8),
            groupsAsync.when(
              loading: () => const Padding(
                padding: EdgeInsets.symmetric(vertical: 8),
                child: LinearProgressIndicator(),
              ),
              error: (_, _) => Text('Could not load groups.', style: TextStyle(color: scheme.onSurfaceVariant)),
              data: (groups) {
                if (groups.isEmpty) {
                  return Text('No groups yet — create them on the web.',
                      style: TextStyle(color: scheme.onSurfaceVariant, fontSize: 13));
                }
                return Wrap(
                  spacing: 8,
                  runSpacing: 6,
                  children: groups
                      .map((g) => FilterChip(
                            label: Text(g.name),
                            selected: _groupIds.contains(g.id),
                            onSelected: (sel) => setState(() {
                              if (sel) {
                                _groupIds.add(g.id);
                              } else {
                                _groupIds.remove(g.id);
                              }
                            }),
                          ))
                      .toList(),
                );
              },
            ),
            if (_err('groups') != null) ...[
              const SizedBox(height: 6),
              Text(_err('groups')!, style: TextStyle(color: scheme.error, fontSize: 12)),
            ],
            const SizedBox(height: 8),
            SwitchListTile.adaptive(
              contentPadding: EdgeInsets.zero,
              title: const Text('Birthday reminders'),
              subtitle: Text('Send reminders for this person', style: TextStyle(color: scheme.onSurfaceVariant, fontSize: 12)),
              value: _notifyBirthdays,
              onChanged: (v) => setState(() => _notifyBirthdays = v),
            ),
            if (_error != null) ...[
              const SizedBox(height: 8),
              Container(
                width: double.infinity,
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
            Row(children: [
              Expanded(
                child: OutlinedButton(
                  onPressed: _saving ? null : () => Navigator.pop(context),
                  child: const Text('Cancel'),
                ),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: FilledButton(
                  onPressed: _saving ? null : _save,
                  child: _saving
                      ? const SizedBox(width: 18, height: 18, child: CircularProgressIndicator(strokeWidth: 2))
                      : Text(_isEdit ? 'Save changes' : 'Create person'),
                ),
              ),
            ]),
          ]),
        ),
      ),
    );
  }
}
