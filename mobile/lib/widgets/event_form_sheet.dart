import 'package:flutter/material.dart';
import '../models/person_detail.dart';
import '../services/api_client.dart';
import '../services/storage_service.dart';
import '../utils/api_errors.dart';
import '../utils/formatters.dart';

/// Create/edit form for a person event, shown as a modal bottom sheet.
///
/// Covers type, date, description and the schedule (frequency, interval,
/// end date, occurrence count) plus the non-Gregorian calendar fields the web
/// form exposes.
class EventFormSheet extends StatefulWidget {
  final int personId;
  final PersonEvent? existing;
  final VoidCallback onSaved;

  const EventFormSheet({super.key, required this.personId, this.existing, required this.onSaved});

  @override
  State<EventFormSheet> createState() => _EventFormSheetState();
}

const _types = <String, String>{
  'birthday': 'Birthday',
  'anniversary': 'Anniversary',
  'wedding': 'Wedding',
  'memorial': 'Memorial',
  'custom': 'Custom',
};

const _calendars = <String, String>{
  'gregorian': 'Gregorian',
  'lunar': 'Lunar',
  'hebrew': 'Hebrew',
  'hijri': 'Hijri',
};

const _frequencies = <String, String>{
  'once': 'Once',
  'annual': 'Every year',
  'monthly': 'Every month',
  'weekly': 'Every week',
  'every_n': 'Every N days',
};

class _EventFormSheetState extends State<EventFormSheet> {
  final _formKey = GlobalKey<FormState>();
  late final TextEditingController _date;
  late final TextEditingController _description;
  late final TextEditingController _notes;
  late final TextEditingController _interval;
  late final TextEditingController _endDate;
  late final TextEditingController _count;
  late String _type;
  late String _calendar;
  late String _frequency;
  late bool _lunarLeap;
  int? _lunarMonth;
  int? _lunarDay;
  bool _saving = false;
  String? _error;
  Map<String, String> _fieldErrors = {};

  bool get _isEdit => widget.existing != null;
  bool get _nonGregorian => _calendar != 'gregorian';

  @override
  void initState() {
    super.initState();
    final e = widget.existing;
    _date = TextEditingController(text: e?.date ?? isoToday());
    _description = TextEditingController(text: e?.description ?? '');
    _notes = TextEditingController(text: e?.notes ?? '');
    _interval = TextEditingController(text: (e?.interval ?? 1).toString());
    _endDate = TextEditingController(text: e?.endDate ?? '');
    _count = TextEditingController(text: e?.occurrenceCount?.toString() ?? '');
    _type = _types.containsKey(e?.type) ? e!.type : 'birthday';
    _calendar = _calendars.containsKey(e?.calendarSystem) ? e!.calendarSystem : 'gregorian';
    _frequency = _frequencies.containsKey(e?.frequency) ? e!.frequency : 'annual';
    _lunarLeap = e?.lunarLeap ?? false;
    _lunarMonth = e?.lunarMonth;
    _lunarDay = e?.lunarDay;
    if (_nonGregorian) _frequency = 'annual';
  }

  @override
  void dispose() {
    _date.dispose();
    _description.dispose();
    _notes.dispose();
    _interval.dispose();
    _endDate.dispose();
    _count.dispose();
    super.dispose();
  }

  String? _err(String field) => _fieldErrors[field];

  Future<void> _pickDate(TextEditingController target) async {
    final now = DateTime.now();
    final initial = DateTime.tryParse(target.text) ?? now;
    final picked = await showDatePicker(
      context: context,
      initialDate: initial,
      firstDate: DateTime(1900),
      lastDate: DateTime(now.year + 10, 12, 31),
    );
    if (picked != null) setState(() => target.text = isoDate(picked));
  }

  Future<void> _save() async {
    setState(() {
      _fieldErrors = {};
      _error = null;
    });
    if (!_formKey.currentState!.validate()) return;

    final countText = _count.text.trim();
    int? count;
    if (countText.isNotEmpty) {
      count = int.tryParse(countText);
      if (count == null || count < 1) {
        setState(() => _fieldErrors = {'occurrence_count': 'Enter a whole number of 1 or more'});
        return;
      }
    }
    final interval = int.tryParse(_interval.text.trim()) ?? 1;

    if (_nonGregorian && (_lunarMonth == null || _lunarDay == null)) {
      setState(() => _fieldErrors = {
            if (_lunarMonth == null) 'lunar_month': 'Select a month',
            if (_lunarDay == null) 'lunar_day': 'Select a day',
          });
      return;
    }

    final body = <String, dynamic>{
      'type': _type,
      'date': _date.text.trim(),
      'description': _description.text.trim(),
      'notes': _notes.text.trim(),
      'calendar_system': _calendar,
      'frequency': _frequency,
      'interval': interval,
      'end_date': _endDate.text.trim(),
      'occurrence_count': count ?? 0,
      if (_nonGregorian) 'lunar_month': _lunarMonth,
      if (_nonGregorian) 'lunar_day': _lunarDay,
      if (_calendar == 'lunar') 'lunar_leap': _lunarLeap,
    };

    setState(() => _saving = true);
    try {
      final storage = StorageService();
      final baseUrl = await storage.getBaseUrl();
      if (baseUrl == null) throw Exception('No server URL');
      final client = ApiClient(storage: storage, baseUrl: baseUrl);
      if (_isEdit) {
        await client.updateEvent(widget.existing!.id, body);
      } else {
        await client.createEvent(widget.personId, body);
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
            Text(_isEdit ? 'Edit event' : 'Add event',
                style: const TextStyle(fontSize: 20, fontWeight: FontWeight.w800)),
            const SizedBox(height: 4),
            Text('Dates repeat every year unless you choose otherwise.',
                style: TextStyle(color: scheme.onSurfaceVariant)),
            const SizedBox(height: 20),
            DropdownButtonFormField<String>(
              initialValue: _type,
              decoration: InputDecoration(
                labelText: 'Type *',
                border: const OutlineInputBorder(),
                errorText: _err('type'),
              ),
              items: _types.entries.map((e) => DropdownMenuItem(value: e.key, child: Text(e.value))).toList(),
              onChanged: (v) => setState(() => _type = v ?? 'birthday'),
            ),
            const SizedBox(height: 12),
            TextFormField(
              controller: _date,
              readOnly: true,
              onTap: () => _pickDate(_date),
              decoration: InputDecoration(
                labelText: 'Date *',
                border: const OutlineInputBorder(),
                suffixIcon: const Icon(Icons.calendar_today_outlined),
                errorText: _err('date'),
              ),
              validator: (v) => v == null || v.trim().isEmpty ? 'Date is required' : null,
            ),
            const SizedBox(height: 12),
            TextFormField(
              controller: _description,
              textCapitalization: TextCapitalization.sentences,
              decoration: InputDecoration(
                labelText: 'Description',
                border: const OutlineInputBorder(),
                errorText: _err('description'),
              ),
            ),
            const SizedBox(height: 12),
            TextFormField(
              controller: _notes,
              maxLines: 2,
              minLines: 1,
              textCapitalization: TextCapitalization.sentences,
              decoration: InputDecoration(
                labelText: 'Notes',
                border: const OutlineInputBorder(),
                errorText: _err('notes'),
              ),
            ),
            const SizedBox(height: 20),
            Text('Calendar', style: TextStyle(fontWeight: FontWeight.w700, color: scheme.onSurface)),
            const SizedBox(height: 8),
            DropdownButtonFormField<String>(
              initialValue: _calendar,
              decoration: const InputDecoration(border: OutlineInputBorder()),
              items: _calendars.entries.map((e) => DropdownMenuItem(value: e.key, child: Text(e.value))).toList(),
              onChanged: (v) => setState(() {
                _calendar = v ?? 'gregorian';
                if (_nonGregorian) _frequency = 'annual';
              }),
            ),
            if (_nonGregorian) ...[
              const SizedBox(height: 12),
              Row(children: [
                Expanded(
                  child: DropdownButtonFormField<int>(
                    key: ValueKey('month-$_calendar'),
                    initialValue: _lunarMonth,
                    decoration: InputDecoration(
                      labelText: 'Month *',
                      border: const OutlineInputBorder(),
                      errorText: _err('lunar_month'),
                    ),
                    items: List.generate(_calendar == 'hebrew' ? 13 : 12, (i) => i + 1)
                        .map((m) => DropdownMenuItem(value: m, child: Text('$m')))
                        .toList(),
                    onChanged: (v) => setState(() => _lunarMonth = v),
                  ),
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: DropdownButtonFormField<int>(
                    initialValue: _lunarDay,
                    decoration: InputDecoration(
                      labelText: 'Day *',
                      border: const OutlineInputBorder(),
                      errorText: _err('lunar_day'),
                    ),
                    items: List.generate(30, (i) => i + 1)
                        .map((d) => DropdownMenuItem(value: d, child: Text('$d')))
                        .toList(),
                    onChanged: (v) => setState(() => _lunarDay = v),
                  ),
                ),
              ]),
              if (_calendar == 'lunar')
                CheckboxListTile(
                  contentPadding: EdgeInsets.zero,
                  title: const Text('Leap month'),
                  value: _lunarLeap,
                  onChanged: (v) => setState(() => _lunarLeap = v ?? false),
                ),
            ],
            const SizedBox(height: 20),
            Text('Repeat', style: TextStyle(fontWeight: FontWeight.w700, color: scheme.onSurface)),
            const SizedBox(height: 8),
            DropdownButtonFormField<String>(
              key: ValueKey('frequency-$_calendar'),
              initialValue: _frequency,
              decoration: InputDecoration(
                border: const OutlineInputBorder(),
                errorText: _err('frequency'),
              ),
              items: _frequencies.entries
                  .map((e) => DropdownMenuItem(
                        value: e.key,
                        // Non-Gregorian calendars only support annual repetition.
                        enabled: !_nonGregorian || e.key == 'annual',
                        child: Text(e.value),
                      ))
                  .toList(),
              onChanged: _nonGregorian ? null : (v) => setState(() => _frequency = v ?? 'annual'),
            ),
            if (_frequency == 'every_n') ...[
              const SizedBox(height: 12),
              TextFormField(
                controller: _interval,
                keyboardType: TextInputType.number,
                decoration: InputDecoration(
                  labelText: 'Every N days',
                  border: const OutlineInputBorder(),
                  errorText: _err('interval'),
                ),
              ),
            ],
            const SizedBox(height: 12),
            Row(children: [
              Expanded(
                child: TextFormField(
                  controller: _endDate,
                  readOnly: true,
                  onTap: () => _pickDate(_endDate),
                  decoration: InputDecoration(
                    labelText: 'End date',
                    border: const OutlineInputBorder(),
                    suffixIcon: _endDate.text.isEmpty
                        ? const Icon(Icons.event_outlined)
                        : IconButton(
                            icon: const Icon(Icons.clear),
                            onPressed: () => setState(() => _endDate.text = ''),
                          ),
                    errorText: _err('end_date'),
                  ),
                ),
              ),
              const SizedBox(width: 12),
              Expanded(
                child: TextFormField(
                  controller: _count,
                  keyboardType: TextInputType.number,
                  decoration: InputDecoration(
                    labelText: 'Occurrences',
                    border: const OutlineInputBorder(),
                    errorText: _err('occurrence_count'),
                  ),
                ),
              ),
            ]),
            if (_error != null) ...[
              const SizedBox(height: 12),
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
                      : Text(_isEdit ? 'Save changes' : 'Create event'),
                ),
              ),
            ]),
          ]),
        ),
      ),
    );
  }
}
