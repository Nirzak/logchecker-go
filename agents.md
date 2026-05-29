# Logchecker — Agent Reference

> **Purpose**: This file gives AI agents a fast, accurate orientation to the `orpheusnet/logchecker` codebase so that they can reason, modify, and extend it without having to re-read the whole source tree.

---

## 1. What the project is

**Logchecker** is a PHP library (and CLI tool) that parses and scores CD-rip log files produced by three rippers:

| Ripper | Constant | Log signature |
|---|---|---|
| Exact Audio Copy (EAC) | `Ripper::EAC` | Contains `"Exact Audio Copy"` |
| X Lossless Decoder (XLD) | `Ripper::XLD` | Contains `"X Lossless Decoder version"` |
| whipper | `Ripper::WHIPPER` | Contains `"Log created by: whipper"` |
| dBpoweramp | `Ripper::DBPOWERAMP` | First line matches `^dBpoweramp Release` |

A score starts at **100** and decreases based on problems found (bad settings, checksum failures, CRC mismatches, etc.). The final score and an array of human-readable detail messages are the primary output.

---

## 2. Repository layout

```
logchecker-fork/
├── bin/
│   ├── logchecker          # Shell wrapper that boots the Symfony Console app
│   └── compile             # Script to build logchecker.phar
├── scripts/
│   └── update_offsets.php  # Downloads fresh drive-offset data from AccurateRip and writes src/resources/drives.json
├── src/
│   ├── Logchecker.php          # Core class — all parsing logic lives here (2193 lines)
│   ├── LogcheckerConsole.php   # Symfony Console Application; registers commands
│   ├── Chardet.php             # Thin wrapper around cchardetect / chardetect / chardet CLI tools
│   ├── Util.php                # Static helpers: commandExists(), decodeEncoding()
│   ├── Check/
│   │   ├── Checksum.php        # CHECKSUM_OK / CHECKSUM_INVALID / CHECKSUM_MISSING + validate()
│   │   └── Ripper.php          # Detects ripper from raw log text; defines EAC / XLD / WHIPPER constants
│   ├── Command/
│   │   ├── AnalyzeCommand.php  # CLI: analyze <file> — primary command
│   │   ├── DecodeCommand.php   # CLI: decode <file> — UTF-8 conversion only
│   │   └── TranslateCommand.php# CLI: translate <file> — EAC non-English → English
│   ├── Exception/
│   │   ├── FileNotFoundException.php
│   │   ├── InvalidFileException.php
│   │   ├── UnknownLanguageException.php
│   │   └── UnknownRipperException.php
│   ├── Parser/
│   │   └── EAC/
│   │       ├── Translator.php      # EAC-only: detects log language, translates to English
│   │       └── languages/
│   │           ├── master.json     # Keyed by language code; contains eac_strings[] used for detection
│   │           ├── en.json         # English "canonical" key→phrase mapping (keys are integers)
│   │           ├── de.json         # German translations (same key schema)
│   │           ├── fr.json / ru.json / jp.json / … (16 other language files)
│   │           └── README.md       # Explains translation file format
│   └── resources/
│       └── drives.json         # ~6 000 drive entries: [[drive_name_lowercase, offset_int], …]
├── tests/
│   ├── LogcheckerTest.php          # Data-provider test against real log fixtures
│   ├── UtilTest.php
│   ├── Check/
│   │   ├── ChecksumTest.php
│   │   └── RipperTest.php
│   ├── Parser/EAC/
│   │   └── TranslatorTest.php
│   └── logs/
│       ├── eac/                # originals/ details/ html/ utf8/
│       ├── xld/                # originals/ details/ html/
│       ├── whipper/            # originals/ details/ html/
│       └── dbpoweramp/         # (reserved / extra fixtures)
├── composer.json               # Package: orpheusnet/logchecker v0.14.4
├── phpunit.xml
├── phpcs.xml
├── phpstan.neon / phpstan-baseline.neon
└── box.json                    # box-project/box config for phar compilation
```

---

## 3. Namespace & autoloading

| Namespace | Maps to |
|---|---|
| `OrpheusNET\Logchecker` | `src/` |
| `OrpheusNET\Logchecker\Check` | `src/Check/` |
| `OrpheusNET\Logchecker\Command` | `src/Command/` |
| `OrpheusNET\Logchecker\Exception` | `src/Exception/` |
| `OrpheusNET\Logchecker\Parser\EAC` | `src/Parser/EAC/` |

PSR-4, configured in `composer.json`.

---

## 4. Core class: `Logchecker`

**File**: [`src/Logchecker.php`](src/Logchecker.php)

### Public API

```php
$lc = new Logchecker();

// Load a file (resets all internal state)
$lc->newFile('/path/to/file.log');

// Run analysis
$lc->parse();

// Results
$lc->getRipper();          // string: 'EAC' | 'XLD' | 'whipper' | 'unknown'
$lc->getRipperVersion();   // string|null
$lc->getScore();           // int 0–100
$lc->getChecksumState();   // 'checksum_ok' | 'checksum_invalid' | 'checksum_missing'
$lc->getDetails();         // string[] — human-readable list of deductions / notices
$lc->getLanguage();        // string language code, e.g. 'en', 'ru'
$lc->getLog();             // string — HTML-annotated log text (span-tagged)
$lc->isCombinedLog();      // bool — true when the file holds multiple rip sessions

// Control
$lc->validateChecksum(false); // Disable external checksum validation

// Static
Logchecker::getAcceptValues();        // ".txt,.TXT,.log,.LOG"
Logchecker::getLogcheckerVersion();   // reads version from composer.json
```

### Parse flow (`parse()`)

```
parse()
 ├── Util::decodeEncoding()          Converts log to UTF-8 (BOM detection + chardet)
 ├── Ripper::getRipper()             Determines ripper type
 ├── if DBPOWERAMP → dbpowerampParse()  Settings + per-track regex parsing
 ├── if WHIPPER → whipperParse()     YAML-based parsing
 └── else → legacyParse()
          ├── if EAC → Translator    Auto-detect + translate non-English to English
          ├── Split log into sections by checksum delimiter or "End of status report"
          ├── Per-section: ~60 preg_replace_callback() calls for each log field
          │    Each callback both annotates the log text with HTML spans
          │    AND calls account() or accountTrack() if a problem is detected
          └── checkTracks() — fails score to 0 if no tracks found
```

### Scoring mechanics

- `account($msg, $decrease, $score, $inclCombined, $notice)` — **global** deduction.  
  - `$decrease` → `$this->Score -= $decrease`  
  - `$score` → `$this->Score = $score` (absolute override)  
  - Deduplication: same message string is never added twice.
- `accountTrack($msg, $decrease)` — **per-track** deduction.  
  - Accumulated into `$this->DecreaseScoreTrack` (applied at end of loop).  
  - Track-level messages carry `"Track NN: …"` prefix.

### HTML annotation classes

The log text returned by `getLog()` contains `<span>` tags used for colorization in web UIs:

| Class | Meaning |
|---|---|
| `good` | Green — expected/correct value |
| `goodish` | Cyan — acceptable but not ideal |
| `badish` | Yellow — suspicious / minor issue |
| `bad` | Red — definite problem |
| `log1` | Underline — version/date info |
| `log3` | Blue — file / technical data |
| `log4` | Bold — field values |
| `log5` | Underline — field labels |

---

## 5. Checksum validation

**File**: [`src/Check/Checksum.php`](src/Check/Checksum.php)

| Ripper | Method | External dependency |
|---|---|---|
| whipper | SHA-256 hash computed in PHP, compared to last line of log | none |
| dBpoweramp | Always `CHECKSUM_MISSING` — no embedded checksum | none |
| EAC | Shells out to `eac_logchecker` (Python) | `pip install eac-logchecker` |
| XLD | Shells out to `xld_logchecker` (Python) | `pip install xld-logchecker` |

`Checksum::logcheckerExists($ripper)` uses `Util::commandExists()` to check whether the external tool is installed. If absent, validation is skipped and the state remains `CHECKSUM_OK` (generous assumption).

---

## 6. Drive offset matching

- `src/resources/drives.json` — JSON array of `[drive_name_lowercase, offset_int]` pairs (~6 000 entries).
- Loaded once in `Logchecker::__construct()`.
- Matching in `getDrives()` uses **Levenshtein distance** (`levenshtein()`) controlled by the constant `LOGCHECKER_LEVENSTEIN_DISTANCE` (default `0` = exact match only; can be set to allow fuzzy matching).
- Drive names are normalised before matching: vendor alias substitutions (e.g. `HL-DT-ST` → `LG Electronics`), whitespace collapse, revision suffix stripping.
- `scripts/update_offsets.php` scrapes `accuraterip.com/driveoffsets.htm` to regenerate `drives.json`.

---

## 7. EAC multi-language support

- **Detection**: `Translator::getLanguage()` scans `master.json` for EAC-specific marker strings unique to each language.
- **Translation**: `Translator::translate()` does regex-replacement of foreign phrases (from `<lang>.json`) with English canonical phrases (from `en.json`). Keys are integers; case-sensitivity differs by key range (`> 16` uses `ui` flags).
- **Supported languages (16)**: bg, cs, de, en, es, fr, it, jp, ko, nl, pl, ru, se, sk, sr, zh.
- A translation notice (`[Notice]`) is added to `Details` but costs no points.

---

## 8. whipper parsing specifics

- Log format is YAML — parsed via `symfony/yaml`.
- Pre-parse fixups handle two known whipper bugs:
  1. Un-escaped YAML strings in `Release`/`Album` fields.
  2. CRCs starting with `0` that `symfony/yaml` would interpret as octal.
- Minimum supported whipper version: **0.7.3** (earlier versions have octal track number bugs).
- Checksum is a SHA-256 of all lines except the last hash line itself.

---

## 9. CLI interface

Bootstrapped by `bin/logchecker` → `LogcheckerConsole` (extends `Symfony\Component\Console\Application`).

| Command | Class | Key options |
|---|---|---|
| `analyze` (alias: `analyse`) | `AnalyzeCommand` | `--html`, `--no_text`, optional `out_file`, optional `details` (JSON) |
| `decode` | `DecodeCommand` | Converts log encoding to UTF-8; prints to stdout or file |
| `translate` | `TranslateCommand` | `--language (-l)` to force language code |

The `analyze` command outputs JSON details to a file (`$lc->getDetails()` etc.) when a `details` argument is given — this is exactly the format expected by test fixtures in `tests/logs/*/details/*.json`.

---

## 10. Testing

```bash
composer test              # runs phpunit
composer lint              # phpcs
composer lint:fix          # phpcbf
composer static-analysis   # phpstan
```

### Test fixture layout (`tests/logs/<ripper>/`)

```
originals/   ← raw .log files fed to Logchecker
details/     ← expected JSON output (ripper, version, language, combined, score, checksum, details[])
html/        ← expected getLog() HTML output
utf8/        ← UTF-8 re-encoded versions (for decode command tests)
```

`LogcheckerTest::testLogchecker()` is a data-provider test that iterates all files in `originals/`, parses them, and asserts equality with the corresponding `details/*.json` and `html/*.log` fixtures.

> **When adding a new test log**: place it in `originals/`, generate the `details/` JSON via  
> `logchecker analyze file.log /dev/null details.json`, and capture `html/` output with  
> `logchecker analyze --html file.log html_file.log`.

---

## 11. Dependencies

### Runtime (`require`)

| Package | Used for |
|---|---|
| `php ^8.1` | Language runtime |
| `ext-iconv` | Encoding conversion in `Util::decodeEncoding()` |
| `ext-mbstring` | UTF-16 BOM detection and conversion |
| `symfony/console ^6\|^7` | CLI framework |
| `symfony/process ^6\|^7` | Running external checkers (eac/xld_logchecker) and chardet |
| `symfony/yaml ^6\|^7` | Parsing whipper YAML logs |

### Dev only

| Package | Purpose |
|---|---|
| `phpunit/phpunit ^10.5` | Testing |
| `phpstan/phpstan ^1.12` | Static analysis |
| `squizlabs/php_codesniffer ^3.8` | Coding standards |

### Optional external tools (Python)

```bash
pip3 install cchardet eac-logchecker xld-logchecker
```

- `cchardetect` / `chardetect` / `chardet` — character encoding detection for EAC logs.
- `eac_logchecker` — validates EAC log checksum cryptographically.
- `xld_logchecker` — validates XLD signature block.

---

## 12. Important constants and tunables

| Constant / key | Default | Location | Effect |
|---|---|---|---|
| `LOGCHECKER_LEVENSTEIN_DISTANCE` | `0` | global (top of `Logchecker.php`) | Max edit distance for drive name fuzzy match |
| `Logchecker::$ValidateChecksum` | `true` | instance | Set via `validateChecksum(false)` to skip external tool |
| `Checksum::CHECKSUM_OK` | `'checksum_ok'` | `Check/Checksum.php` | Returned when checksum passes or tool absent |
| `Checksum::CHECKSUM_INVALID` | `'checksum_invalid'` | `Check/Checksum.php` | Returned when tool reports tampered log |
| `Checksum::CHECKSUM_MISSING` | `'checksum_missing'` | `Check/Checksum.php` | No checksum present in log |

---

## 13. Key scoring deductions (representative list)

| Condition | Points |
|---|---|
| Unknown log / corrupt encoding | −100 (score → 0) |
| EAC version older than 0.99 | −30 |
| Range rip detected | −30 |
| CRC mismatch per track | −30 |
| No test-and-copy used | −10 |
| Rip not in Secure mode AND no T+C | −40 additional |
| Accurate stream not Yes | −20 |
| C2 pointers used | −10 |
| Defeat audio cache not Yes | −10 |
| Incorrect gap handling | −10 |
| Normalization active | −100 (score → 0) |
| Virtual / fake drive used | −20 |
| Incorrect read offset | −5 |
| ID3 tags added to FLAC | −1 |
| Suspicious/timing position per track | −20 |
| Read error per track (capped 10) | −1 to −10 |

---

## 14. Patterns to follow when modifying

1. **Adding a new check**: Add a `preg_replace_callback()` call in `legacyParse()` or `whipperParse()`. The callback should both return an HTML-annotated string AND call `$this->account()` / `$this->accountTrack()` if a problem is detected.

2. **Adding a new language**: Create `src/Parser/EAC/languages/<code>.json` using the same integer-keyed schema as `en.json`, and register a detection string in `master.json`.

3. **Adding a new drive**: Run `scripts/update_offsets.php` to regenerate `src/resources/drives.json` from AccurateRip — do not hand-edit the large JSON file.

4. **Adding a CLI command**: Create a class in `src/Command/`, extend `Symfony\Component\Console\Command\Command`, and register it in `LogcheckerConsole::__construct()`.

5. **Updating test fixtures**: After any scoring logic change, regenerate the `details/` and `html/` fixture files using the `analyze` command, then commit them alongside the code change.

6. **Pre-commit hooks** (configured via `composer.json` `extra.hooks`): `phpcbf` (lint fix) and `phpstan` run automatically on commit.
