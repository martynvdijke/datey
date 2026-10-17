import 'package:flutter/material.dart';
import '../utils/formatters.dart';

class CountdownBadge extends StatelessWidget {
  final int daysRemaining;
  const CountdownBadge({super.key, required this.daysRemaining});

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    Color bg;
    Color fg;
    if (daysRemaining == 0) {
      bg = scheme.tertiaryContainer;
      fg = scheme.onTertiaryContainer;
    } else if (daysRemaining <= 7) {
      bg = scheme.primaryContainer;
      fg = scheme.onPrimaryContainer;
    } else {
      bg = scheme.surfaceContainerHighest;
      fg = scheme.onSurfaceVariant;
    }
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 5),
      decoration: BoxDecoration(color: bg, borderRadius: BorderRadius.circular(20)),
      child: Text(
        countdownText(daysRemaining),
        style: TextStyle(fontSize: 12, fontWeight: FontWeight.w700, color: fg),
      ),
    );
  }
}

class LunarBadge extends StatelessWidget {
  final String label;
  final bool isLeap;
  const LunarBadge({super.key, required this.label, this.isLeap = false});

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
      decoration: BoxDecoration(
        color: scheme.secondaryContainer,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: scheme.outlineVariant),
      ),
      child: Row(mainAxisSize: MainAxisSize.min, children: [
        Icon(Icons.brightness_2, size: 12, color: scheme.onSecondaryContainer),
        const SizedBox(width: 4),
        Text(
          isLeap ? '$label · Leap' : label,
          style: TextStyle(fontSize: 11, fontWeight: FontWeight.w600, color: scheme.onSecondaryContainer),
        ),
      ]),
    );
  }
}

class MilestoneBadge extends StatelessWidget {
  final String label;
  const MilestoneBadge({super.key, required this.label});

  @override
  Widget build(BuildContext context) {
    final scheme = Theme.of(context).colorScheme;
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
      decoration: BoxDecoration(
        gradient: LinearGradient(colors: [scheme.primary, scheme.tertiary]),
        borderRadius: BorderRadius.circular(12),
      ),
      child: Row(mainAxisSize: MainAxisSize.min, children: [
        const Icon(Icons.celebration, size: 12, color: Colors.white),
        const SizedBox(width: 4),
        Text(label, style: const TextStyle(fontSize: 11, fontWeight: FontWeight.w800, color: Colors.white)),
      ]),
    );
  }
}
