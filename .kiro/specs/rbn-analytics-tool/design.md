# Design Document

## Overview

The RBN Callsign Correction Analytics Tool is a batch-processing Python application that analyzes millions of RBN spot records to infer true callsigns and learn decoder error patterns. The system operates in three main passes:

1. **Pass A - Micro-clustering**: Groups spots by time/frequency proximity and determines provisional true callsigns via skimmer consensus
2. **Pass B - Stability Analysis**: Validates provisional callsigns across larger frequency bins to improve accuracy
3. **Pass C - Confusion & Priors**: Builds character-level error statistics and global callsign frequency counts

The outputs (confusion_model.json and priors.json) are consumed by a separate Go-based runtime correction engine.

## Architecture

The system follows a pipeline architecture with clear separation of concerns:

```
Input CSVs → Load & Filter → Micro-cluster → Stability → Confusion/Priors → JSON Export
```

### Module Structure

```
rbn_train/
├── __init__.py
├── cli.py              # CLI orchestration and main pipeline
├── config.py           # Configuration management
├── io.py               # CSV loading and path resolution
├── types.py            # Data structures (Spot, Cluster, etc.)
├── clustering.py       # Pass A: micro-clustering and provisional truth
├── stability.py        # Pass B: stability bins and truth refinement
├── confusion.py        # Pass C: confusion model building
└── priors.py           # Pass C: global prior computation
```

### Data Flow

1. CLI resolves input paths (file/directory/glob) → list of CSV paths
2. IO module loads and filters spots from all CSVs → List[Spot]
3. Clustering module groups spots → List[Cluster] → List[LabeledCluster]
4. Stability module validates → Dict[StabilityKey, StableBin] → List[FinalCluster]
5. Confusion/Priors modules analyze → ConfusionCounts + Priors
6. Export modules write JSON files

## Components and Interfaces

### CLI Module (cli.py)

**Responsibilities:**
- Parse command-line arguments
- Load and finalize configuration
- Orchestrate the full pipeline
- Log progress metrics
- Write output files

**Key Functions:**
- `main()`: Entry point that orchestrates the entire pipeline
- Logging at each major step (spots loaded, clusters created, etc.)

**CLI Arguments:**
- `--input` (required): File path, directory, or glob pattern
- `--output-dir` (required): Directory for JSON outputs
- `--config` (optional): Path to JSON/YAML config file

### Configuration Module (config.py)

**Responsibilities:**
- Define AnalysisConfig dataclass
- Load configuration from JSON/YAML
- Apply CLI overrides
- Finalize defaults (SNR bands, modes)
- Validate configuration

**AnalysisConfig Fields:**
```python
@dataclass
class AnalysisConfig:
    # Micro-cluster parameters
    cluster_time_seconds: int = 60
    cluster_freq_bin_hz: int = 500
    min_cluster_skimmers: int = 4
    min_cluster_share_percent: float = 70.0
    
    # Stability parameters
    stability_freq_bin_hz: int = 1000
    stability_min_clusters: int = 5
    stability_min_share_percent: float = 80.0
    
    # Confusion model parameters
    snr_bands: Optional[List[float]] = None
    modes: Optional[List[str]] = None
    
    # Character set
    charset: str = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789/"
    unknown_char: str = "?"
    
    # Data filtering
    min_snr_db: float = -999.0
    max_call_length: int = 16
    min_call_length: int = 2
```

**Key Functions:**
- `load_config(path: Optional[str]) -> AnalysisConfig`
- `apply_cli_overrides(config: AnalysisConfig, args: Namespace) -> AnalysisConfig`
- `finalize_config(config: AnalysisConfig) -> AnalysisConfig`

**Default Values:**
- SNR bands: `[-999.0, 0.0, 6.0, 12.0, 18.0, 24.0, 999.0]` (6 bands)
- Modes: `["CW", "RTTY", "SSB"]`

### IO Module (io.py)

**Responsibilities:**
- Resolve input paths from CLI argument
- Load CSV files in chunks
- Parse and normalize spot data
- Filter invalid/unwanted spots

**Key Functions:**

`resolve_input_paths(input_arg: str) -> List[Path]`
- If directory: return all *.csv files (non-recursive)
- If glob pattern: expand and return matches
- Otherwise: return single file path
- Raise error if result is empty

`load_spots(paths: List[Path], config: AnalysisConfig) -> List[Spot]`
- Use pandas.read_csv with chunksize=100,000
- Validate required columns exist
- Parse timestamp to UTC datetime
- Compute freq_hz (from freq_hz or freq_khz * 1000)
- Normalize: uppercase mode/dx_call, strip whitespace
- Filter by: mode in config.modes, call length, SNR threshold, non-empty fields
- Return combined list of Spot objects

### Types Module (types.py)

**Data Structures:**

```python
@dataclass
class Spot:
    timestamp: datetime
    freq_hz: float
    band: str
    mode: str
    skimmer: str
    dx_call: str
    snr_db: float

@dataclass(frozen=True)
class ClusterKey:
    band: str
    mode: str
    freq_bin: int
    time_bin: int

@dataclass
class Cluster:
    key: ClusterKey
    spots: List[Spot]

@dataclass
class LabeledCluster:
    cluster: Cluster
    provisional_true_call: Optional[str]

@dataclass(frozen=True)
class StabilityKey:
    band: str
    mode: str
    freq_bin: int

@dataclass
class StableBin:
    key: StabilityKey
    stable_call: Optional[str]

@dataclass
class FinalCluster:
    cluster: Cluster
    true_call_final: Optional[str]
```

### Clustering Module (clustering.py)

**Responsibilities:**
- Build micro-clusters from spots
- Determine provisional true callsign per cluster via skimmer consensus

**Key Functions:**

`build_micro_clusters(spots: List[Spot], config: AnalysisConfig) -> List[Cluster]`
- For each spot, compute:
  - `freq_bin = int(spot.freq_hz) // config.cluster_freq_bin_hz`
  - `time_bin = int(spot.timestamp.timestamp()) // config.cluster_time_seconds`
- Group spots by ClusterKey(band, mode, freq_bin, time_bin)
- Return list of Cluster objects

`label_provisional_truth(clusters: List[Cluster], config: AnalysisConfig) -> List[LabeledCluster]`
- For each cluster:
  - Build mapping: `call → {skimmers: set, count: int}`
  - Count distinct skimmers per call
  - Compute total_skimmers (union of all skimmer sets)
  - Find best_call (most distinct skimmers)
  - Check for ties
  - Validate thresholds:
    - `best_skimmers >= min_cluster_skimmers`
    - `(100 * best_skimmers / total_skimmers) >= min_cluster_share_percent`
  - Set provisional_true_call if all conditions met, else None
- Return list of LabeledCluster objects

### Stability Module (stability.py)

**Responsibilities:**
- Build larger frequency bins for stability analysis
- Determine stable callsigns per bin
- Refine provisional truth to final truth using stability

**Key Functions:**

`build_stability_bins(labeled_clusters: List[LabeledCluster], config: AnalysisConfig) -> Dict[StabilityKey, List[LabeledCluster]]`
- Skip clusters with provisional_true_call == None
- For each cluster:
  - Compute center_freq_hz (average of spot frequencies)
  - `freq_bin = int(center_freq_hz) // config.stability_freq_bin_hz`
  - Create StabilityKey(band, mode, freq_bin)
  - Group clusters by key
- Return dictionary mapping keys to cluster lists

`compute_stable_calls(bins: Dict[StabilityKey, List[LabeledCluster]], config: AnalysisConfig) -> Dict[StabilityKey, StableBin]`
- For each bin:
  - Count frequency of each provisional_true_call
  - Find best_call (most frequent)
  - Check for ties
  - Validate thresholds:
    - `best_count >= stability_min_clusters`
    - `(100 * best_count / total_clusters) >= stability_min_share_percent`
  - Set stable_call if all conditions met, else None
- Return dictionary mapping keys to StableBin objects

`refine_truth(labeled_clusters: List[LabeledCluster], stable_bins: Dict[StabilityKey, StableBin], config: AnalysisConfig) -> List[FinalCluster]`
- For each cluster:
  - If provisional_true_call is None: true_call_final = None
  - Lookup stability bin using center frequency
  - If no stable_bin or stable_call is None: true_call_final = provisional_true_call
  - If stable_call == provisional_true_call: true_call_final = provisional_true_call
  - Else:
    - Get decoded_calls (set of dx_call from spots)
    - If stable_call in decoded_calls: true_call_final = stable_call
    - Else compute Levenshtein distances
    - If any distance <= 2: true_call_final = stable_call
    - Else: true_call_final = provisional_true_call
- Return list of FinalCluster objects

`levenshtein_distance(a: str, b: str) -> int`
- Standard DP implementation with unit costs
- Returns minimum edit distance

### Confusion Module (confusion.py)

**Responsibilities:**
- Define alphabet and SNR band indexing
- Align true vs observed callsigns
- Build confusion count matrices
- Export confusion_model.json

**Key Classes:**

```python
@dataclass
class Alphabet:
    chars: List[str]
    index_by_char: Dict[str, int]
    unknown_char: str
    unknown_index: int

class EditOpKind(Enum):
    MATCH = "match"
    SUB = "sub"
    DEL = "del"
    INS = "ins"

@dataclass
class EditOp:
    kind: EditOpKind
    true_char: Optional[str]
    obs_char: Optional[str]

class ConfusionCounts:
    sub_counts: np.ndarray  # (num_modes, num_bands, num_chars, num_chars)
    del_counts: np.ndarray  # (num_modes, num_bands, num_chars)
    ins_counts: np.ndarray  # (num_modes, num_bands, num_chars)
```

**Key Functions:**

`build_alphabet(charset: str, unknown_char: str) -> Alphabet`
- Create character list from charset
- Add unknown_char if not present
- Build index mapping

`snr_band_index(snr: float, edges: List[float]) -> int`
- Find band i where `edges[i] < snr <= edges[i+1]`
- Return band index

`normalize_call(call: str, alphabet: Alphabet) -> str`
- Replace characters not in alphabet with unknown_char
- Return normalized string

`align_calls(true_call: str, obs_call: str) -> List[EditOp]`
- Build DP matrix for Levenshtein distance
- Backtrack to extract edit operations
- Return operations in left-to-right order

`build_confusion_and_priors(final_clusters: List[FinalCluster], config: AnalysisConfig) -> (ConfusionCounts, Priors, Alphabet)`
- Initialize alphabet, mode index, confusion counts, priors
- For each FinalCluster with true_call_final:
  - Normalize true_call_final
  - Increment priors count
  - For each spot in cluster:
    - Get mode and SNR band indices
    - Normalize observed dx_call
    - Align true vs observed
    - For each edit operation:
      - Increment appropriate count (sub/del/ins)
- Return confusion, priors, alphabet

`export_confusion_model(confusion: ConfusionCounts, alphabet: Alphabet, modes: List[str], snr_bands: List[float], output_path: Path) -> None`
- Build JSON structure with modes, snr_band_edges, alphabet, unknown_char
- Convert numpy arrays to lists using .tolist()
- Write to file

### Priors Module (priors.py)

**Responsibilities:**
- Accumulate global callsign counts
- Export priors.json

**Key Classes:**

```python
class Priors:
    counts: Dict[str, int]
```

**Key Functions:**

`export_priors(priors: Priors, output_path: Path) -> None`
- Build JSON structure with calls mapping
- Write to file

## Data Models

### Spot
Represents a single RBN observation with timestamp, frequency, band, mode, skimmer ID, decoded callsign, and SNR.

### Cluster Hierarchy
- **Cluster**: Raw grouping of spots by time/frequency
- **LabeledCluster**: Cluster with provisional_true_call from skimmer consensus
- **FinalCluster**: Cluster with true_call_final after stability refinement

### Stability Structures
- **StabilityKey**: Identifies a larger frequency bin for validation
- **StableBin**: Contains the stable_call for a frequency bin

### Confusion Model
- **Alphabet**: Character set with indexing
- **EditOp**: Single edit operation (match/sub/del/ins)
- **ConfusionCounts**: Multi-dimensional arrays of error counts

## Correctness Properties

*A property is a characteristic or behavior that should hold true across all valid executions of a system-essentially, a formal statement about what the system should do. Properties serve as the bridge between human-readable specifications and machine-verifiable correctness guarantees.*

### Property 1: CSV Loading Preserves Valid Data
*For any* valid CSV row with all required columns, loading and filtering should either include the row as a Spot (if it passes filters) or exclude it (if it fails filters), but never corrupt the data values.
**Validates: Requirements 1.1, 1.5, 1.6, 2.1, 2.2, 2.3**

### Property 2: Frequency Conversion Consistency
*For any* spot with freq_khz value, converting to freq_hz by multiplying by 1000.0 should produce the same result as if freq_hz was provided directly with that value.
**Validates: Requirements 1.7**

### Property 3: Cluster Key Determinism
*For any* two spots with identical band, mode, and timestamps/frequencies that fall in the same bins, they should produce identical ClusterKey values and be grouped into the same cluster.
**Validates: Requirements 3.1, 3.2, 3.3, 3.4**

### Property 4: Provisional Truth Threshold Enforcement
*For any* cluster where the best callsign has fewer than min_cluster_skimmers distinct skimmers OR less than min_cluster_share_percent of total skimmers, the provisional_true_call should be None.
**Validates: Requirements 4.4, 4.5**

### Property 5: Stability Bin Grouping Consistency
*For any* two labeled clusters with the same band, mode, and center frequencies that fall in the same stability bin, they should be grouped into the same StabilityKey.
**Validates: Requirements 5.1, 5.2, 5.3**

### Property 6: Stable Call Threshold Enforcement
*For any* stability bin where the best callsign appears in fewer than stability_min_clusters OR has less than stability_min_share_percent share, the stable_call should be None.
**Validates: Requirements 5.5, 5.7, 5.8**

### Property 7: Truth Refinement Levenshtein Boundary
*For any* cluster where stable_call differs from provisional_true_call and stable_call is not in decoded_calls, if all decoded calls have Levenshtein distance > 2 from stable_call, then true_call_final should equal provisional_true_call.
**Validates: Requirements 6.5, 6.6, 6.7**

### Property 8: Levenshtein Alignment Round-Trip
*For any* two callsigns, aligning them to get edit operations and then applying those operations to transform the first callsign should produce the second callsign.
**Validates: Requirements 13.3, 13.4, 13.5, 13.6, 13.7, 13.8, 13.9**

### Property 9: Confusion Model Character Normalization
*For any* callsign containing characters not in the alphabet, normalizing it should replace all unknown characters with unknown_char while preserving known characters.
**Validates: Requirements 7.6**

### Property 10: Confusion Counts Non-Negative
*For any* confusion model built from valid data, all substitution, deletion, and insertion counts should be non-negative integers.
**Validates: Requirements 7.3, 7.4, 7.5, 7.9, 7.10, 7.11**

### Property 11: Prior Counts Consistency
*For any* set of final clusters, the sum of all prior counts should equal the number of clusters with non-None true_call_final values.
**Validates: Requirements 8.1, 8.2, 8.3**

### Property 12: JSON Export Round-Trip
*For any* confusion model exported to JSON and then re-imported, the array dimensions and structure should match the original (modes, SNR bands, alphabet size).
**Validates: Requirements 9.1, 9.2, 9.3, 9.4, 9.5**

## Error Handling

### Input Validation Errors
- **Missing required columns**: Log error with missing column names and skip the file
- **Invalid timestamp format**: Skip rows with unparseable timestamps, log warning with row number
- **Missing frequency data**: Skip rows without freq_hz or freq_khz, log warning
- **Empty input path result**: Terminate with clear error message indicating no CSV files found

### Configuration Errors
- **Invalid unknown_char**: Raise ValueError if unknown_char is not a single character
- **Empty modes list**: Use default modes ["CW", "RTTY", "SSB"]
- **Empty SNR bands**: Use default band edges
- **Non-existent output directory**: Create directory automatically

### Data Processing Errors
- **Empty cluster**: Handle gracefully by setting provisional_true_call to None
- **Tie in skimmer counts**: Set provisional_true_call to None
- **Tie in stability bin**: Set stable_call to None
- **Missing stability bin**: Fall back to provisional_true_call

### Export Errors
- **Cannot write output file**: Log error with full path and exception details
- **Numpy array conversion failure**: Log error and attempt to continue with remaining exports

## Testing Strategy

### Unit Testing Approach

The system will use pytest for unit testing with focus on:

1. **Utility Functions**
   - `snr_band_index()` with various SNR values and edge cases
   - `build_alphabet()` with different charsets and unknown_char values
   - `normalize_call()` with mixed valid/invalid characters
   - `levenshtein_distance()` with known examples

2. **Data Transformation**
   - CSV row parsing and normalization
   - Frequency conversion (freq_khz to freq_hz)
   - Cluster key generation from spots

3. **Business Logic**
   - Provisional truth determination with various skimmer distributions
   - Stability bin grouping and stable call selection
   - Truth refinement logic with different Levenshtein distances

4. **Edge Cases**
   - Empty clusters
   - Ties in voting
   - Missing data (None values)
   - Boundary conditions for thresholds

### Property-Based Testing Approach

The system will use Hypothesis for property-based testing to verify universal properties across many randomly generated inputs.

**Configuration:**
- Minimum 100 iterations per property test
- Each property test must reference its corresponding design property using the format: `# Feature: rbn-analytics-tool, Property N: <property text>`

**Key Properties to Test:**

1. **Property 1: CSV Loading Preserves Valid Data**
   - Generate random valid CSV rows
   - Verify data values are preserved or correctly filtered

2. **Property 3: Cluster Key Determinism**
   - Generate random spots with controlled bin values
   - Verify identical bins produce identical keys

3. **Property 8: Levenshtein Alignment Round-Trip**
   - Generate random callsign pairs
   - Verify alignment operations correctly transform source to target

4. **Property 9: Confusion Model Character Normalization**
   - Generate random strings with mixed valid/invalid characters
   - Verify normalization preserves valid chars and replaces invalid ones

5. **Property 10: Confusion Counts Non-Negative**
   - Generate random final clusters
   - Verify all counts are >= 0

6. **Property 11: Prior Counts Consistency**
   - Generate random final clusters
   - Verify sum of priors equals count of non-None true_call_final

**Test Data Generators:**
- Random spots with configurable parameters
- Random clusters with varying skimmer distributions
- Random callsigns with controlled character sets
- Random SNR values across band boundaries

### Integration Testing

While the focus is on unit and property tests, key integration points to validate:

1. **End-to-End Pipeline**
   - Small synthetic CSV → full pipeline → verify JSON outputs exist and are valid
   - Verify output JSON structure matches expected schema

2. **Module Boundaries**
   - IO → Clustering: spots correctly grouped
   - Clustering → Stability: labeled clusters correctly refined
   - Stability → Confusion: final clusters correctly analyzed

### Performance Testing

Not part of automated test suite, but manual validation should confirm:
- 10M+ spots processed in reasonable time (< 30 minutes on modern hardware)
- Memory usage stays reasonable with chunked CSV reading
- No memory leaks during long runs 