<!-- file: docs/reports/relink-manual-fixes-result-2026-04-29.md -->
<!-- version: 1.0.0 -->
<!-- guid: a1b2c3d4-e5f6-7890-abcd-ef1234567891 -->

# iTunes Relink Manual Fix Results — 2026-04-29

## Applied Fixes

| Book ID | Title | Result |
|---------|-------|--------|
| 01KNDBMNN5GHYA272PWXTRBD8H | Orion Colony: An Intergalactic Space Opera Adventure | OK |
| 01KNDBRK6R1BN2R7RJ1M65R33T | Predator: Stalking Shadows | OK |
| 01KNDBT1FHH7TV6Q9X4N0MXTS2 | Level Up!: Checkpoint | OK |
| 01KNDBTJYR5QRQ3QMDX0ZYA6Y3 | Night Angel Nemesis | SKIP — not found in iTunes (see Not-Found section) |
| 01KNDBWPTSVZKD6DERNW84JE0G | Old Guns: A Military Sci-Fi Adventure | OK |
| 01KNDC0VX7HJV9KKQ6PN843E6C | The Wastes: Underdog Series, Book 2 | OK |
| 01KNDC15NNJSKFTRM5A1JJZWQV | Page Keeper 3: A Slice of Life Fantasy | OK |
| 01KNDC4QQYG6MQFM75JH0VTKMG | Oaths and Outfits: A Superhero Slice-of-Life LitRPG | OK |
| 01KNDCAAEPWTMA67QX2FT13JM1 | Nighthawk: Sons of de Wolfe | OK |
| 01KNDCAGS9XZTV7NYBT5WWKFR8 | Ninth House | SKIP — not found in iTunes (see Not-Found section) |
| 01KNDCBPQHE53JQSNCKG63GNJV | Promises Kept | SKIP — not found in iTunes (see Not-Found section) |
| 01KNDCBT2ZTY48K2VSFR9H738Z | Portal Wars - 2 - The Ten Thousand | SKIP — not found in iTunes (see Not-Found section) |
| 01KNDCGBNF2ET83NRQ5S2N4A79 | The Book of Riley 3 | OK (mapped to Part 3 per part-number scheme) |

## Not-Found Books — Search Output

### Night Angel Nemesis

Search command to run on production server:
```
find '/mnt/bigdata/books/itunes' -iname '*night angel nemesis*' 2>/dev/null
```

Not executed — book confirmed absent from iTunes per report. No file found in any Brent Weeks directory. Needs manual sourcing or decision to leave unlinked.

### Ninth House

Search command to run on production server:
```
find '/mnt/bigdata/books/itunes' -iname '*ninth house*' 2>/dev/null
```

Not executed — book confirmed absent from iTunes per report. No Leigh Bardugo directory exists in iTunes. Needs manual sourcing or decision to leave unlinked.

### Promises Kept

Search command to run on production server:
```
find '/mnt/bigdata/books/itunes' -iname '*promise*' 2>/dev/null | grep -i anderle
```

Not executed — book confirmed absent from iTunes per report. No standalone Michael Anderle directory in iTunes; book not found across ~25 co-author directories. Needs manual sourcing or decision to leave unlinked.

### Portal Wars - 2 - The Ten Thousand

Search command to run on production server:
```
find '/mnt/bigdata/books/itunes' -iname '*jay allan*' 2>/dev/null
```

Not executed — book confirmed absent from iTunes per report. No Jay Allan directory in iTunes. Needs manual sourcing or decision to leave unlinked.

## Summary

- Fixed: 9
- Skipped (not in iTunes — manual resolution needed): 4
- Errors: 0
