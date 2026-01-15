// Package telnet implements the multi-client telnet server for the DX Cluster.
//
// The telnet server is the primary user interface, handling:
//   - Client connections and authentication (callsign-based login)
//   - Real-time spot broadcasting to all connected clients
//   - Per-client filtering (band, mode, callsign patterns)
//   - User command processing (HELP, SHOW/DX, SHOW/STATION, BYE)
//   - Telnet protocol handling (IAC sequences, line ending conversion)
//
// Architecture:
//   - One goroutine per connected client (handleClient)
//   - Broadcast goroutine for distributing spots to all clients
//   - Non-blocking spot delivery (full channels don't block the system)
//   - Each client has their own Filter instance for personalized feeds
//
// Client Session Flow:
//  1. Client connects → Welcome message sent
//  2. Prompt for callsign → Client enters callsign
//  3. Login complete → Client receives greeting and help
//  4. Command loop → Process commands and broadcast spots
//  5. Client types BYE or disconnects → Session ends
//
// Concurrency Design:
//   - clientsMu protects the clients map (add/remove operations)
//   - Each client goroutine operates independently
//   - Broadcast uses non-blocking sends to avoid slow client blocking
//   - Graceful degradation: Full spot channels result in dropped spots for that client
//
// Maximum concurrent connections: Configurable (typically 500)
package telnet

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"dxcluster/commands"
	"dxcluster/cty"
	"dxcluster/filter"
	"dxcluster/pathreliability"
	"dxcluster/reputation"
	"dxcluster/spot"
	ztelnet "github.com/ziutek/telnet"
)

// Server represents a multi-client telnet server for DX Cluster connections.
//
// The server maintains a map of connected clients and broadcasts spots to all clients
// in real-time. Each client has its own goroutine for handling commands and receiving spots.
//
// Fields:
//   - port: TCP port to listen on (configured via `telnet.port` in `config.yaml`)
//   - welcomeMessage: Initial message sent to connecting clients
//   - maxConnections: Maximum concurrent client connections (typically 500)
//   - listener: TCP listener for accepting new connections
//   - clients: Map of callsign → Client for all connected clients
//   - clientsMutex: Read-write mutex protecting the clients map
//   - shutdown: Channel for coordinating graceful shutdown
//   - broadcast: Buffered channel for spot broadcasting (capacity 100)
//   - processor: Command processor for handling user commands
//
// Thread Safety:
//   - Start() and Stop() can be called from any goroutine
//   - BroadcastSpot() is thread-safe (uses mutex)
//   - Each client goroutine operates independently
type Server struct {
	port              int                         // TCP port to listen on
	welcomeMessage    string                      // Welcome message for new connections
	maxConnections    int                         // Maximum concurrent client connections
	duplicateLoginMsg string                      // Message sent to evicted duplicate session
	greetingTemplate  string                      // Post-login greeting with placeholders
	clusterCall       string                      // Cluster/node callsign for greeting substitution
	listener          net.Listener                // TCP listener
	clients           map[string]*Client          // Map of callsign → Client
	clientsMutex      sync.RWMutex                // Protects clients map
	shutdown          chan struct{}               // Shutdown coordination channel
	broadcast         chan *spot.Spot             // Broadcast channel for spots (buffered, configurable)
	broadcastWorkers  int                         // Number of goroutines delivering spots
	workerQueues      []chan *broadcastJob        // Per-worker job queues
	workerQueueSize   int                         // Capacity of each worker's queue
	batchInterval     time.Duration               // Broadcast batch interval; 0 means immediate
	batchMax          int                         // Max jobs per batch before flush
	metrics           broadcastMetrics            // Broadcast metrics counters
	keepaliveInterval time.Duration               // Optional periodic CRLF to keep idle sessions alive
	clientShards      [][]*Client                 // Cached shard layout for broadcasts
	shardsDirty       atomic.Bool                 // Flag to rebuild shards on client add/remove
	processor         *commands.Processor         // Command processor for user commands
	skipHandshake     bool                        // When true, omit Telnet IAC negotiation
	transport         string                      // Telnet transport backend ("native" or "ziutek")
	useZiutek         bool                        // True when the external telnet transport is enabled
	echoMode          string                      // Input echo policy ("server", "local", "off")
	clientBufferSize  int                         // Per-client spot channel capacity
	loginLineLimit    int                         // Maximum bytes accepted for login/callsign input
	commandLineLimit  int                         // Maximum bytes accepted for post-login commands
	filterEngine      *filterCommandEngine        // Table-driven filter command parser/executor
	reputationGate    *reputation.Gate            // Optional reputation gate for login metadata
	startTime         time.Time                   // Process start time for uptime tokens
	pathPredictor     *pathreliability.Predictor  // Optional path reliability predictor
	pathDisplay       bool                        // Toggle glyph rendering
	noiseOffsets      map[string]float64          // Noise class lookup
	gridLookup        func(string) (string, bool) // Optional grid lookup from store
}

// Client represents a connected telnet client session.
//
// Each client has:
//   - Dedicated goroutine for handling commands and receiving spots
//   - Personal Filter for customizing which spots they receive
//   - Buffered spot channel for non-blocking spot delivery
//   - Telnet protocol handling (IAC sequence processing)
//
// The client remains active until they type BYE or their connection drops.
type Client struct {
	conn         net.Conn               // TCP connection to client
	reader       *bufio.Reader          // Buffered reader for client input
	writer       *bufio.Writer          // Buffered writer for client output
	callsign     string                 // Client's amateur radio callsign
	connected    time.Time              // Timestamp when client connected
	server       *Server                // Back-reference to server for formatting/helpers
	address      string                 // Client's IP address
	recentIPs    []string               // Most-recent-first IP history for this callsign
	spotChan     chan *spot.Spot        // Buffered channel for spot delivery (configurable capacity)
	bulletinChan chan bulletin          // Buffered channel for WWV/WCY bulletin delivery
	handleIAC    bool                   // True when we parse IAC sequences in ReadLine
	echoInput    bool                   // True when we should echo typed characters back to the client
	dialect      DialectName            // Active command dialect for filter commands
	grid         string                 // User grid (4+ chars) for path reliability
	gridCell     pathreliability.CellID // Cached cell for path reliability
	noiseClass   string                 // Noise class token (e.g., QUIET, URBAN)
	noisePenalty float64                // dB penalty applied DX->user

	// filterMu guards filter, which is read by telnet broadcast workers while the
	// client session goroutine mutates it in response to PASS/REJECT commands.
	// Without this lock, Go's runtime can terminate the process with:
	// "fatal error: concurrent map read and map write"
	// because Filter contains many maps.
	filterMu          sync.RWMutex
	filter            *filter.Filter // Personal spot filter (band, mode, callsign)
	dropCount         uint64         // Count of spots dropped for this client due to backpressure
	pendingDeliveries sync.WaitGroup // Outstanding broadcast jobs referencing this client
}

// InputValidationError represents a non-fatal ingress violation (length or character guardrails).
// Returning this error allows the caller to keep the connection open and prompt the user again.
type InputValidationError struct {
	reason      string
	userMessage string
}

func (e *InputValidationError) Error() string {
	return e.reason
}

// UserMessage returns the friendly text that should be sent back to the telnet client.
func (e *InputValidationError) UserMessage() string {
	if e == nil || strings.TrimSpace(e.userMessage) == "" {
		return "Input rejected. Please try again."
	}
	return e.userMessage
}

func (c *Client) saveFilter() error {
	if c == nil || c.filter == nil {
		return nil
	}
	callsign := strings.TrimSpace(c.callsign)
	if callsign == "" {
		return nil
	}
	// Persisting the filter marshals multiple maps; guard with a read lock so it
	// cannot run concurrently with PASS/REJECT updates. Broadcast workers also
	// hold read locks while matching, so persistence does not stall spot delivery.
	c.filterMu.RLock()
	defer c.filterMu.RUnlock()
	record := &filter.UserRecord{
		Filter:     *c.filter,
		RecentIPs:  c.recentIPs,
		Dialect:    string(c.dialect),
		Grid:       strings.ToUpper(strings.TrimSpace(c.grid)),
		NoiseClass: strings.ToUpper(strings.TrimSpace(c.noiseClass)),
	}
	if existing, err := filter.LoadUserRecord(callsign); err == nil {
		record.RecentIPs = filter.MergeRecentIPs(record.RecentIPs, existing.RecentIPs)
	}
	if err := filter.SaveUserRecord(callsign, record); err != nil {
		log.Printf("Warning: failed to save user record for %s: %v", callsign, err)
		return err
	}
	log.Printf("Saved user record for %s", callsign)
	return nil
}

// updateFilter applies a mutation to the per-client Filter while holding the
// write lock, protecting against concurrent reads from broadcast workers.
func (c *Client) updateFilter(fn func(f *filter.Filter)) {
	if c == nil || c.filter == nil || fn == nil {
		return
	}
	c.filterMu.Lock()
	fn(c.filter)
	c.filterMu.Unlock()
}

type broadcastJob struct {
	spot    *spot.Spot
	clients []*Client
}

type bulletin struct {
	kind string
	line string
}

type broadcastMetrics struct {
	queueDrops  uint64
	clientDrops uint64
}

func (m *broadcastMetrics) snapshot() (queueDrops, clientDrops uint64) {
	queueDrops = atomic.LoadUint64(&m.queueDrops)
	clientDrops = atomic.LoadUint64(&m.clientDrops)
	return
}

// handleDialectCommand lets a client select a filter command dialect explicitly.
func (s *Server) handleDialectCommand(client *Client, line string) (string, bool) {
	if client == nil || s == nil || s.filterEngine == nil {
		return "", false
	}
	fields := strings.Fields(strings.TrimSpace(line))
	if len(fields) == 0 {
		return "", false
	}
	if !strings.EqualFold(fields[0], "DIALECT") {
		return "", false
	}
	if len(fields) == 1 {
		return fmt.Sprintf("Current dialect: %s\n", strings.ToUpper(string(client.dialect))), true
	}
	if len(fields) == 2 && strings.EqualFold(fields[1], "LIST") {
		names := s.filterEngine.availableDialectNames()
		return fmt.Sprintf("Available dialects: %s\n", strings.ToUpper(strings.Join(names, ", "))), true
	}
	nameToken := strings.ToLower(strings.TrimSpace(fields[1]))
	selected, ok := s.filterEngine.dialectAliases[nameToken]
	if !ok {
		// Also accept exact dialect names.
		if dialect := DialectName(nameToken); s.filterEngine.dialects[dialect] != nil {
			selected = dialect
			ok = true
		}
	}
	if !ok {
		return fmt.Sprintf("Unknown dialect: %s\nAvailable dialects: %s\n", fields[1], strings.ToUpper(strings.Join(s.filterEngine.availableDialectNames(), ", "))), true
	}
	if client.dialect == selected {
		return fmt.Sprintf("Dialect already set to %s\n", strings.ToUpper(string(selected))), true
	}
	client.dialect = selected
	// Persist updated dialect selection.
	_ = client.saveFilter()
	return fmt.Sprintf("Dialect set to %s\n", strings.ToUpper(string(selected))), true
}

func dialectWelcomeLine(active DialectName, created bool, loadErr error, defaultDialect DialectName) string {
	source := "default"
	if loadErr == nil && !created {
		source = "persisted"
	}
	if active != defaultDialect {
		source = "persisted"
	}
	return fmt.Sprintf("Current dialect: %s (%s). Use DIALECT LIST or DIALECT CC to switch. Type HELP for commands in this dialect.\n", strings.ToUpper(string(active)), source)
}

// handlePathSettingsCommand processes SET GRID/SET NOISE commands.
func (s *Server) handlePathSettingsCommand(client *Client, line string) (string, bool) {
	if client == nil {
		return "", false
	}
	upper := strings.Fields(strings.ToUpper(strings.TrimSpace(line)))
	if len(upper) < 2 || upper[0] != "SET" {
		return "", false
	}
	switch upper[1] {
	case "GRID":
		if len(upper) < 3 {
			return "Usage: SET GRID <4-6 char maidenhead>\n", true
		}
		grid := strings.ToUpper(strings.TrimSpace(upper[2]))
		if len(grid) < 4 {
			return "Grid must be at least 4 characters (e.g., FN31)\n", true
		}
		cell := pathreliability.EncodeCell(grid)
		if cell == pathreliability.InvalidCell {
			return "Invalid grid. Please provide a 4-6 character Maidenhead locator.\n", true
		}
		client.grid = grid
		client.gridCell = cell
		if err := client.saveFilter(); err != nil {
			return fmt.Sprintf("Grid set to %s (warning: failed to persist: %v)\n", grid, err), true
		}
		return fmt.Sprintf("Grid set to %s\n", grid), true
	case "NOISE":
		if len(upper) < 3 {
			return "Usage: SET NOISE <QUIET|RURAL|SUBURBAN|URBAN>\n", true
		}
		class := strings.ToUpper(strings.TrimSpace(upper[2]))
		penalty := s.noisePenaltyForClass(class)
		if class == "" || (penalty == 0 && class != "QUIET" && class != "RURAL" && class != "SUBURBAN" && class != "URBAN") {
			return "Unknown noise class. Use QUIET, RURAL, SUBURBAN, or URBAN.\n", true
		}
		client.noiseClass = class
		client.noisePenalty = penalty
		if err := client.saveFilter(); err != nil {
			return fmt.Sprintf("Noise class set to %s (warning: failed to persist: %v)\n", class, err), true
		}
		return fmt.Sprintf("Noise class set to %s\n", class), true
	default:
		return "", false
	}
}

func shouldLogQueueDrop(total uint64) bool {
	return total == 1 || total%100 == 0
}

// Telnet protocol IAC (Interpret As Command) constants.
//
// These constants are used for telnet protocol negotiation:
//   - IAC: Introduces a telnet command sequence
//   - DO/DONT: Request client to enable/disable an option
//   - WILL/WONT: Client agrees/refuses to enable an option
//
// The server sends these sequences to negotiate terminal settings
// and handle special characters properly.
const (
	IAC  = 255 // Interpret As Command - starts telnet command sequence
	DONT = 254 // Request client to disable an option
	DO   = 253 // Request client to enable an option
	WONT = 252 // Client refuses to enable an option
	WILL = 251 // Client agrees to enable an option
)

const (
	telnetEchoServer = "server"
	telnetEchoLocal  = "local"
	telnetEchoOff    = "off"
)

const (
	defaultBroadcastQueueSize     = 2048
	defaultBroadcastBatch         = 512
	defaultBroadcastBatchInterval = 250 * time.Millisecond
	defaultClientBufferSize       = 128
	defaultWorkerQueueSize        = 128
	defaultDuplicateLoginMessage  = "Another login for your callsign connected. This session is being closed (multiple logins are not allowed)."
	defaultSendDeadline           = 2 * time.Second
	defaultLoginLineLimit         = 32
	defaultCommandLineLimit       = 128
)

const (
	passFilterUsageMsg      = "Usage: PASS <type> ...\nPASS BAND <band>[,<band>...] | PASS MODE <mode>[,<mode>...] | PASS SOURCE <HUMAN|SKIMMER|ALL> | PASS DXCALL <pattern> | PASS DECALL <pattern> | PASS CONFIDENCE <symbol>[,<symbol>...] (symbols: ?,S,C,P,V,B or ALL) | PASS BEACON | PASS WWV | PASS WCY | PASS ANNOUNCE | PASS DXGRID2 <grid>[,<grid>...] (two characters or ALL) | PASS DEGRID2 <grid>[,<grid>...] (two characters or ALL) | PASS DXCONT <cont>[,<cont>...] | PASS DECONT <cont>[,<cont>...] | PASS DXZONE <zone>[,<zone>...] | PASS DEZONE <zone>[,<zone>...] | PASS DXDXCC <code>[,<code>...] | PASS DEDXCC <code>[,<code>...] (PASS = allow list; clears block-all). Supported modes include: CW, LSB, USB, JS8, SSTV, RTTY, FT4, FT8, MSK144, PSK.\nType HELP for usage.\n"
	rejectFilterUsageMsg    = "Usage: REJECT <type> ...\nREJECT ALL | REJECT BAND <band>[,<band>...] | REJECT MODE <mode>[,<mode>...] | REJECT SOURCE <HUMAN|SKIMMER> | REJECT DXCALL (clears all patterns; args ignored) | REJECT DECALL (clears all patterns; args ignored) | REJECT CONFIDENCE <symbol>[,<symbol>...] (symbols: ?,S,C,P,V,B or ALL) | REJECT BEACON | REJECT WWV | REJECT WCY | REJECT ANNOUNCE | REJECT DXGRID2 <grid>[,<grid>...] (two characters or ALL) | REJECT DEGRID2 <grid>[,<grid>...] (two characters or ALL) | REJECT DXCONT <cont>[,<cont>...] | REJECT DECONT <cont>[,<cont>...] | REJECT DXZONE <zone>[,<zone>...] | REJECT DEZONE <zone>[,<zone>...] | REJECT DXDXCC <code>[,<code>...] | REJECT DEDXCC <code>[,<code>...] (REJECT = block list; ALL resets to defaults). Supported modes include: CW, LSB, USB, JS8, SSTV, RTTY, FT4, FT8, MSK144, PSK.\nType HELP for usage.\n"
	unknownPassTypeMsg      = "Unknown filter type. Use: BAND, MODE, SOURCE, DXCALL, DECALL, CONFIDENCE, BEACON, WWV, WCY, ANNOUNCE, DXGRID2, DEGRID2, DXCONT, DECONT, DXZONE, DEZONE, DXDXCC, or DEDXCC\nType HELP for usage.\n"
	unknownRejectTypeMsg    = "Unknown filter type. Use: ALL, BAND, MODE, SOURCE, DXCALL, DECALL, CONFIDENCE, BEACON, WWV, WCY, ANNOUNCE, DXGRID2, DEGRID2, DXCONT, DECONT, DXZONE, DEZONE, DXDXCC, or DEDXCC\nType HELP for usage.\n"
	invalidFilterCommandMsg = "Invalid filter command. Type HELP for usage.\n"
)

// ServerOptions configures the telnet server instance.
type ServerOptions struct {
	Port                   int
	WelcomeMessage         string
	DuplicateLoginMsg      string
	LoginGreeting          string
	ClusterCall            string
	MaxConnections         int
	BroadcastWorkers       int
	BroadcastQueue         int
	WorkerQueue            int
	ClientBuffer           int
	BroadcastBatchInterval time.Duration
	KeepaliveSeconds       int
	SkipHandshake          bool
	Transport              string
	EchoMode               string
	LoginLineLimit         int
	CommandLineLimit       int
	ReputationGate         *reputation.Gate
	PathPredictor          *pathreliability.Predictor
	PathDisplayEnabled     bool
	NoiseOffsets           map[string]float64
	GridLookup             func(string) (string, bool)
	CTYLookup              func() *cty.CTYDatabase
}

// NewServer creates a new telnet server
func NewServer(opts ServerOptions, processor *commands.Processor) *Server {
	config := normalizeServerOptions(opts)
	useZiutek := strings.EqualFold(config.Transport, "ziutek")
	return &Server{
		port:              config.Port,
		welcomeMessage:    config.WelcomeMessage,
		maxConnections:    config.MaxConnections,
		duplicateLoginMsg: config.DuplicateLoginMsg,
		greetingTemplate:  config.LoginGreeting,
		clusterCall:       config.ClusterCall,
		clients:           make(map[string]*Client),
		shutdown:          make(chan struct{}),
		broadcast:         make(chan *spot.Spot, config.BroadcastQueue),
		broadcastWorkers:  config.BroadcastWorkers,
		workerQueueSize:   config.WorkerQueue,
		batchInterval:     config.BroadcastBatchInterval,
		batchMax:          defaultBroadcastBatch,
		keepaliveInterval: time.Duration(config.KeepaliveSeconds) * time.Second,
		clientBufferSize:  config.ClientBuffer,
		skipHandshake:     config.SkipHandshake,
		transport:         config.Transport,
		useZiutek:         useZiutek,
		echoMode:          config.EchoMode,
		processor:         processor,
		loginLineLimit:    config.LoginLineLimit,
		commandLineLimit:  config.CommandLineLimit,
		filterEngine:      newFilterCommandEngineWithCTY(config.CTYLookup),
		reputationGate:    opts.ReputationGate,
		startTime:         time.Now().UTC(),
		pathPredictor:     opts.PathPredictor,
		pathDisplay:       opts.PathDisplayEnabled,
		noiseOffsets:      opts.NoiseOffsets,
		gridLookup:        opts.GridLookup,
	}
}

func normalizeServerOptions(opts ServerOptions) ServerOptions {
	config := opts
	if config.BroadcastWorkers <= 0 {
		config.BroadcastWorkers = defaultBroadcastWorkers()
	}
	if config.BroadcastQueue <= 0 {
		config.BroadcastQueue = defaultBroadcastQueueSize
	}
	if config.WorkerQueue <= 0 {
		config.WorkerQueue = defaultWorkerQueueSize
	}
	if config.ClientBuffer <= 0 {
		config.ClientBuffer = defaultClientBufferSize
	}
	if config.BroadcastBatchInterval <= 0 {
		config.BroadcastBatchInterval = defaultBroadcastBatchInterval
	}
	if strings.TrimSpace(config.DuplicateLoginMsg) == "" {
		config.DuplicateLoginMsg = defaultDuplicateLoginMessage
	}
	if strings.TrimSpace(config.LoginGreeting) == "" {
		config.LoginGreeting = "Hello <CALL>, you are now connected to <CLUSTER>. Type HELP for available commands."
	}
	if strings.TrimSpace(config.ClusterCall) == "" {
		config.ClusterCall = "DXC"
	}
	if strings.TrimSpace(config.Transport) == "" {
		config.Transport = "native"
	}
	config.Transport = strings.ToLower(strings.TrimSpace(config.Transport))
	if strings.TrimSpace(config.EchoMode) == "" {
		config.EchoMode = "server"
	}
	config.EchoMode = strings.ToLower(strings.TrimSpace(config.EchoMode))
	if config.LoginLineLimit <= 0 {
		config.LoginLineLimit = defaultLoginLineLimit
	}
	if config.CommandLineLimit <= 0 {
		config.CommandLineLimit = defaultCommandLineLimit
	}
	if config.PathPredictor == nil {
		config.PathDisplayEnabled = false
	}
	if config.NoiseOffsets == nil {
		config.NoiseOffsets = map[string]float64{}
	}
	return config
}

// Start begins listening for telnet connections
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.port)
	listener, err := listenWithReuse(addr)
	if err != nil {
		return fmt.Errorf("failed to start telnet server: %w", err)
	}

	s.listener = listener
	log.Printf("Telnet server listening on port %d", s.port)

	// Prepare worker pool before handling spots
	s.startWorkerPool()

	// Start broadcast handler
	go s.handleBroadcasts()

	// Optional keepalive emitter for idle sessions.
	if s.keepaliveInterval > 0 {
		go s.keepaliveLoop()
	}

	// Accept connections in a goroutine
	go s.acceptConnections()

	return nil
}

// listenWithReuse enables SO_REUSEADDR so we can rebind quickly after a crash/exit.
// It falls back to a standard Listen when the control call fails.
func listenWithReuse(addr string) (net.Listener, error) {
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			var sockErr error
			controlErr := c.Control(func(fd uintptr) {
				sockErr = setReuseAddr(fd)
			})
			if controlErr != nil {
				return controlErr
			}
			return sockErr
		},
	}
	listener, err := lc.Listen(context.Background(), "tcp", addr)
	if err != nil {
		// Fallback to default listener to avoid failing on platforms that reject the control call.
		return net.Listen("tcp", addr)
	}
	return listener, nil
}

// handleBroadcasts sends spots to all connected clients
func (s *Server) handleBroadcasts() {
	for {
		select {
		case <-s.shutdown:
			return
		case spot := <-s.broadcast:
			s.broadcastSpot(spot)
		}
	}
}

// keepaliveLoop emits periodic CRLF to all connected clients to prevent idle
// disconnects by intermediate network devices when the spot stream is quiet.
func (s *Server) keepaliveLoop() {
	ticker := time.NewTicker(s.keepaliveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.shutdown:
			return
		case <-ticker.C:
			s.clientsMutex.RLock()
			for _, client := range s.clients {
				_ = client.Send("\r\n")
			}
			s.clientsMutex.RUnlock()
		}
	}
}

// BroadcastSpot sends a spot to all connected clients
func (s *Server) BroadcastSpot(spot *spot.Spot) {
	select {
	case s.broadcast <- spot:
	default:
		drops := atomic.AddUint64(&s.metrics.queueDrops, 1)
		if shouldLogQueueDrop(drops) {
			log.Printf("Broadcast channel full (%d/%d buffered), dropping spot (total queue drops=%d)", len(s.broadcast), cap(s.broadcast), drops)
		}
	}
}

// BroadcastRaw sends a raw line to all connected clients without formatting.
// Use this for non-spot PC frames (e.g., PC26) that should pass through unchanged.
func (s *Server) BroadcastRaw(line string) {
	if s == nil || strings.TrimSpace(line) == "" {
		return
	}
	s.clientsMutex.RLock()
	defer s.clientsMutex.RUnlock()
	for _, client := range s.clients {
		_ = client.Send(line)
	}
}

// BroadcastWWV sends a WWV/WCY bulletin to all connected clients that allow it.
// The kind must be WWV, WCY, PC23, or PC73; unknown kinds are ignored.
func (s *Server) BroadcastWWV(kind string, line string) {
	kind = normalizeWWVKind(kind)
	if kind == "" {
		return
	}
	s.broadcastBulletin(kind, line, true)
}

// BroadcastAnnouncement sends a PC93 announcement to all connected clients
// that allow announcements.
func (s *Server) BroadcastAnnouncement(line string) {
	s.broadcastBulletin("ANNOUNCE", line, true)
}

// SendDirectMessage sends a PC93 talk message to a specific callsign when connected.
func (s *Server) SendDirectMessage(callsign string, line string) {
	if s == nil {
		return
	}
	callsign = strings.ToUpper(strings.TrimSpace(callsign))
	if callsign == "" {
		return
	}
	message := prepareBulletinLine(line)
	if message == "" {
		return
	}
	s.clientsMutex.RLock()
	client := s.clients[callsign]
	s.clientsMutex.RUnlock()
	if client == nil || client.bulletinChan == nil {
		return
	}
	s.enqueueBulletin(client, "TALK", message)
}

func (s *Server) broadcastBulletin(kind string, line string, applyFilter bool) {
	if s == nil {
		return
	}
	message := prepareBulletinLine(line)
	if message == "" {
		return
	}
	s.clientsMutex.RLock()
	defer s.clientsMutex.RUnlock()
	for _, client := range s.clients {
		if client.bulletinChan == nil {
			continue
		}
		if applyFilter {
			client.filterMu.RLock()
			allowed := client.filter.AllowsBulletin(kind)
			client.filterMu.RUnlock()
			if !allowed {
				continue
			}
		}
		s.enqueueBulletin(client, kind, message)
	}
}

func (s *Server) enqueueBulletin(client *Client, kind, message string) {
	if client == nil || client.bulletinChan == nil {
		return
	}
	select {
	case client.bulletinChan <- bulletin{kind: kind, line: message}:
	default:
		drops := atomic.AddUint64(&s.metrics.clientDrops, 1)
		clientDrops := atomic.AddUint64(&client.dropCount, 1)
		if shouldLogQueueDrop(drops) {
			log.Printf("Client %s bulletin channel full, dropping %s bulletin (client drops=%d total=%d)", client.callsign, kind, clientDrops, drops)
		}
	}
}

func prepareBulletinLine(line string) string {
	trimmed := strings.TrimRight(line, "\r\n")
	if strings.TrimSpace(trimmed) == "" {
		return ""
	}
	return trimmed + "\n"
}

func normalizeWWVKind(kind string) string {
	kind = strings.ToUpper(strings.TrimSpace(kind))
	switch kind {
	case "WWV", "WCY":
		return kind
	case "PC23":
		return "WWV"
	case "PC73":
		return "WCY"
	default:
		return ""
	}
}

// broadcastSpot segments clients into shards and enqueues jobs for workers
func (s *Server) broadcastSpot(spot *spot.Spot) {
	shards := s.cachedClientShards()
	s.dispatchSpotToWorkers(spot, shards)
}

func (s *Server) startWorkerPool() {
	if s.broadcastWorkers <= 0 {
		s.broadcastWorkers = defaultBroadcastWorkers()
	}
	if len(s.workerQueues) != 0 {
		return
	}
	queueSize := s.workerQueueSize
	if queueSize <= 0 {
		queueSize = defaultWorkerQueueSize
	}
	s.workerQueues = make([]chan *broadcastJob, s.broadcastWorkers)
	for i := 0; i < s.broadcastWorkers; i++ {
		s.workerQueues[i] = make(chan *broadcastJob, queueSize)
		go s.broadcastWorker(i, s.workerQueues[i])
	}
}

func (s *Server) dispatchSpotToWorkers(spot *spot.Spot, shards [][]*Client) {
	for i, clients := range shards {
		if len(clients) == 0 {
			continue
		}
		for _, client := range clients {
			client.pendingDeliveries.Add(1)
		}
		job := &broadcastJob{spot: spot, clients: clients}
		select {
		case s.workerQueues[i] <- job:
		default:
			drops := atomic.AddUint64(&s.metrics.queueDrops, 1)
			if shouldLogQueueDrop(drops) {
				log.Printf("Worker %d queue full (%d pending jobs), dropping %d-client shard (total queue drops=%d)", i, len(s.workerQueues[i]), len(clients), drops)
			}
			for _, client := range clients {
				client.pendingDeliveries.Done()
			}
		}
	}
}

// cachedClientShards returns the shard snapshot, rebuilding only when marked dirty.
func (s *Server) cachedClientShards() [][]*Client {
	if !s.shardsDirty.Load() && len(s.clientShards) > 0 {
		return s.clientShards
	}

	s.clientsMutex.RLock()
	defer s.clientsMutex.RUnlock()

	workers := s.broadcastWorkers
	if workers <= 0 {
		workers = 1
	}
	shards := make([][]*Client, workers)
	idx := 0
	for _, client := range s.clients {
		shard := idx % workers
		shards[shard] = append(shards[shard], client)
		idx++
	}
	s.clientShards = shards
	s.shardsDirty.Store(false)
	return shards
}

func (s *Server) broadcastWorker(id int, jobs <-chan *broadcastJob) {
	log.Printf("Broadcast worker %d started", id)
	// Immediate mode when batching is disabled.
	if s.batchInterval <= 0 {
		for {
			select {
			case <-s.shutdown:
				return
			case job := <-jobs:
				if job == nil {
					continue
				}
				s.deliverJob(job)
			}
		}
	}

	ticker := time.NewTicker(s.batchInterval)
	defer ticker.Stop()
	batch := make([]*broadcastJob, 0, s.batchMax)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		for _, job := range batch {
			if job == nil {
				continue
			}
			s.deliverJob(job)
		}
		batch = batch[:0]
	}

	for {
		select {
		case <-s.shutdown:
			flush()
			return
		case job := <-jobs:
			if job == nil {
				continue
			}
			batch = append(batch, job)
			if len(batch) >= s.batchMax {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func (s *Server) deliverJob(job *broadcastJob) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("telnet: panic delivering broadcast job: %v", r)
		}
	}()
	for _, client := range job.clients {
		if client == nil {
			continue
		}
		client.pendingDeliveries.Done()
		// Always deliver spots where the DX callsign matches the logged-in user,
		// regardless of filters (self-spots should not be filtered out).
		if !strings.EqualFold(job.spot.DXCall, client.callsign) {
			client.filterMu.RLock()
			matches := client.filter.Matches(job.spot)
			client.filterMu.RUnlock()
			if !matches {
				continue
			}
		}
		select {
		case client.spotChan <- job.spot:
		default:
			drops := atomic.AddUint64(&s.metrics.clientDrops, 1)
			clientDrops := atomic.AddUint64(&client.dropCount, 1)
			if shouldLogQueueDrop(drops) {
				log.Printf("Client %s spot channel full, dropping spot (client drops=%d total=%d)", client.callsign, clientDrops, drops)
			}
		}
	}
}

// WorkerCount returns the number of broadcast workers currently configured.
func (s *Server) WorkerCount() int {
	workers := s.broadcastWorkers
	if workers <= 0 {
		workers = defaultBroadcastWorkers()
	}
	return workers
}

func (s *Server) BroadcastMetricSnapshot() (queueDrops, clientDrops uint64) {
	return s.metrics.snapshot()
}

func defaultBroadcastWorkers() int {
	workers := runtime.NumCPU()
	if workers < 4 {
		workers = 4
	}
	return workers
}

// acceptConnections handles incoming connections
func (s *Server) acceptConnections() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.shutdown:
				// Server is shutting down
				return
			default:
				log.Printf("Error accepting connection: %v", err)
				continue
			}
		}

		// Enforce configured connection limit before spinning up a client goroutine.
		if s.maxConnections > 0 {
			addr := conn.RemoteAddr().String()
			s.clientsMutex.RLock()
			current := len(s.clients)
			s.clientsMutex.RUnlock()
			if current >= s.maxConnections {
				_, _ = conn.Write([]byte("Server full. Try again later.\r\n"))
				conn.Close()
				log.Printf("Rejected connection from %s: max connections reached (%d)", addr, s.maxConnections)
				continue
			}
		}
		if tcp, ok := conn.(*net.TCPConn); ok {
			_ = tcp.SetKeepAlive(true)
			_ = tcp.SetKeepAlivePeriod(2 * time.Minute)
		}

		// Handle this client in a new goroutine
		go s.handleClient(conn)
	}
}

// handleClient manages a single client connection
func (s *Server) handleClient(conn net.Conn) {
	defer conn.Close()

	address := conn.RemoteAddr().String()
	log.Printf("New connection from %s", address)

	spotQueueSize := s.clientBufferSize
	if spotQueueSize <= 0 {
		spotQueueSize = defaultClientBufferSize
	}

	if s.filterEngine == nil {
		s.filterEngine = newFilterCommandEngine()
	}

	// Create client object and select the telnet transport backend.
	readerConn := conn
	writerConn := conn
	if s.useZiutek {
		tconn, err := ztelnet.NewConn(conn)
		if err != nil {
			log.Printf("telnet: failed to wrap connection from %s: %v", address, err)
			return
		}
		readerConn = tconn
		writerConn = tconn
	}
	client := &Client{
		conn:         conn,
		reader:       bufio.NewReader(readerConn),
		writer:       bufio.NewWriter(writerConn),
		connected:    time.Now(),
		server:       s,
		address:      address,
		spotChan:     make(chan *spot.Spot, spotQueueSize),
		bulletinChan: make(chan bulletin, spotQueueSize),
		filter:       filter.NewFilter(), // Start with no filters (accept all)
		handleIAC:    !s.useZiutek,
		dialect:      s.filterEngine.defaultDialect,
		gridCell:     pathreliability.InvalidCell,
		noiseClass:   "QUIET",
		noisePenalty: 0,
		// Echo policy is configured explicitly so we can support local echo even
		// when clients toggle their own modes.
		echoInput: s.echoMode == telnetEchoServer,
	}

	s.negotiateTelnet(client)

	// Send welcome message with template tokens (uptime, user count, etc.).
	loginTime := time.Now().UTC()
	client.Send(applyTemplateTokens(s.welcomeMessage, s.preLoginTemplateData(loginTime)))
	client.Send("\r\nEnter your callsign:\r\n")

	var callsign string
	for {
		// Read callsign with tight guard rails so a single telnet client cannot
		// consume unbounded memory or smuggle control characters during login. The
		// limit is configurable but defaults to 32 bytes which covers every valid
		// callsign, including suffixes such as /QRP or /MM.
		line, err := client.ReadLine(s.loginLineLimit, "login", false, false, false, false)
		if err != nil {
			var inputErr *InputValidationError
			if errors.As(err, &inputErr) {
				client.Send(inputErr.UserMessage() + "\n")
				client.Send("Enter your callsign:\r\n")
				continue
			}
			log.Printf("Error reading callsign from %s: %v", address, err)
			return
		}

		line = strings.ToUpper(strings.TrimSpace(line))
		if line == "" {
			client.Send("Callsign cannot be empty. Please try again.\n")
			client.Send("Enter your callsign:\r\n")
			continue
		}
		normalized := spot.NormalizeCallsign(line)
		if !spot.IsValidNormalizedCallsign(normalized) {
			client.Send("Invalid callsign. Please try again.\n")
			client.Send("Enter your callsign:\r\n")
			continue
		}
		callsign = normalized
		break
	}

	client.callsign = callsign
	log.Printf("Client %s logged in as %s", address, client.callsign)

	if s.reputationGate != nil {
		s.reputationGate.RecordLogin(client.callsign, spotterIP(client.address), time.Now().UTC())
	}

	// Capture the client's IP immediately after login so it is persisted before
	// any other session state mutates.
	record, created, prevLogin, prevIP, err := filter.TouchUserRecordLogin(client.callsign, spotterIP(client.address), loginTime)
	if err == nil {
		client.filter = &record.Filter
		client.recentIPs = record.RecentIPs
		client.dialect = normalizeDialectName(record.Dialect)
		client.grid = strings.ToUpper(strings.TrimSpace(record.Grid))
		client.gridCell = pathreliability.EncodeCell(client.grid)
		client.noiseClass = strings.ToUpper(strings.TrimSpace(record.NoiseClass))
		client.noisePenalty = s.noisePenaltyForClass(client.noiseClass)
		if created {
			log.Printf("Created default filter for %s", client.callsign)
		} else {
			log.Printf("Loaded saved filter for %s", client.callsign)
		}
	} else {
		client.filter = filter.NewFilter()
		client.recentIPs = filter.UpdateRecentIPs(nil, spotterIP(client.address))
		client.dialect = s.filterEngine.defaultDialect
		client.gridCell = pathreliability.InvalidCell
		client.noiseClass = "QUIET"
		client.noisePenalty = s.noisePenaltyForClass(client.noiseClass)
		log.Printf("Warning: failed to load user record for %s: %v", client.callsign, err)
		if err := client.saveFilter(); err != nil {
			log.Printf("Warning: failed to save user record for %s: %v", client.callsign, err)
		}
	}

	// Seed grid from lookup when none is stored.
	if strings.TrimSpace(client.grid) == "" && s.gridLookup != nil {
		if g, ok := s.gridLookup(client.callsign); ok {
			client.grid = strings.ToUpper(strings.TrimSpace(g))
			client.gridCell = pathreliability.EncodeCell(client.grid)
		}
	}
	// Normalize noise defaults when absent.
	if strings.TrimSpace(client.noiseClass) == "" {
		client.noiseClass = "QUIET"
		client.noisePenalty = s.noisePenaltyForClass(client.noiseClass)
	}

	// Register client
	s.registerClient(client)
	defer s.unregisterClient(client)

	// Send login confirmation
	greeting := formatGreeting(s.greetingTemplate, s.postLoginTemplateData(loginTime, client, prevLogin, prevIP))
	if strings.TrimSpace(greeting) == "" {
		greeting = fmt.Sprintf("Hello %s, you are now connected.", client.callsign)
	}
	client.Send(greeting + "\n")
	client.Send(dialectWelcomeLine(client.dialect, created, err, s.filterEngine.defaultDialect))
	if s.pathPredictor != nil && s.pathDisplay {
		status := "Path reliability enabled"
		gridNote := client.grid
		if gridNote == "" {
			gridNote = "unset"
		}
		client.Send(fmt.Sprintf("%s. Grid: %s (SET GRID <grid>), Noise: %s (SET NOISE QUIET|RURAL|SUBURBAN|URBAN)\n", status, gridNote, client.noiseClass))
	}

	// Start spot sender goroutine
	go client.spotSender()

	// Read commands from client
	for {
		// Commands use the more relaxed limit because filter manipulation can
		// legitimately include several tokens. The limit is still kept small
		// (default 128 bytes) to keep parsing cheap and predictable.
		line, err := client.ReadLine(s.commandLineLimit, "command", true, true, true, true)
		if err != nil {
			var inputErr *InputValidationError
			if errors.As(err, &inputErr) {
				client.Send(inputErr.UserMessage() + "\n")
				continue
			}
			log.Printf("Client %s disconnected: %v", client.callsign, err)
			return
		}

		// Treat blank lines as client keepalives: echo CRLF so idle clients see traffic.
		if strings.TrimSpace(line) == "" {
			_ = client.Send("\r\n")
			continue
		}

		// Dialect selection is handled before filter commands.
		if resp, handled := s.handleDialectCommand(client, line); handled {
			if resp != "" {
				client.Send(resp)
			}
			continue
		}

		if resp, handled := s.handlePathSettingsCommand(client, line); handled {
			if resp != "" {
				client.Send(resp)
			}
			continue
		}

		// Check for filter commands under the active dialect.
		if resp, handled := s.filterEngine.Handle(client, line); handled {
			if resp != "" {
				client.Send(resp)
			}
			continue
		}

		// Process other commands
		filterFn := func(s *spot.Spot) bool {
			if s == nil {
				return false
			}
			if strings.EqualFold(s.DXCall, client.callsign) {
				return true
			}
			client.filterMu.RLock()
			f := client.filter
			matches := f != nil && f.Matches(s)
			client.filterMu.RUnlock()
			return matches
		}
		response := s.processor.ProcessCommandForClient(line, client.callsign, spotterIP(client.address), filterFn, string(client.dialect))

		// Check for disconnect signal
		if response == "BYE" {
			client.Send("73!\n")
			log.Printf("Client %s logged out", client.callsign)
			return
		}

		// Send response
		if response != "" {
			client.Send(response)
		}
	}
}

// negotiateTelnet performs minimal option negotiation to keep echo behavior
// predictable across telnet clients. It writes directly to the raw connection
// to avoid IAC escaping by higher-level telnet transports.
func (s *Server) negotiateTelnet(client *Client) {
	if s.skipHandshake || client == nil || client.conn == nil {
		return
	}
	conn := client.conn
	// Prefer full-duplex sessions by suppressing go-ahead.
	sendTelnetOption(conn, WILL, 3)
	sendTelnetOption(conn, DO, 3)

	switch s.echoMode {
	case telnetEchoServer:
		// Server will echo input; ask the client to disable local echo.
		sendTelnetOption(conn, WILL, 1)
		sendTelnetOption(conn, DONT, 1)
	case telnetEchoLocal:
		// Server will not echo; most clients enable local echo in response.
		sendTelnetOption(conn, WONT, 1)
	case telnetEchoOff:
		// Best-effort: request no echo from either side.
		sendTelnetOption(conn, WONT, 1)
		sendTelnetOption(conn, DONT, 1)
	default:
		// Fall back to server echo if the mode is unknown.
		sendTelnetOption(conn, WILL, 1)
		sendTelnetOption(conn, DONT, 1)
	}
}

func sendTelnetOption(conn net.Conn, command, option byte) {
	if conn == nil {
		return
	}
	if err := conn.SetWriteDeadline(time.Now().Add(defaultSendDeadline)); err != nil {
		return
	}
	_, _ = conn.Write([]byte{IAC, command, option})
	_ = conn.SetWriteDeadline(time.Time{})
}

// templateData holds contextual values that can be substituted into operator-configured templates.
type templateData struct {
	now       time.Time
	startTime time.Time
	userCount int
	callsign  string
	cluster   string
	lastLogin time.Time
	lastIP    string
}

// formatGreeting replaces placeholders using the provided template data.
func formatGreeting(tmpl string, data templateData) string {
	if tmpl == "" {
		return ""
	}
	return applyTemplateTokens(tmpl, data)
}

func (s *Server) noisePenaltyForClass(class string) float64 {
	if s == nil {
		return 0
	}
	key := strings.ToUpper(strings.TrimSpace(class))
	if val, ok := s.noiseOffsets[key]; ok {
		return val
	}
	return 0
}

// formatSpotForClient renders a spot with optional reliability glyphs.
func (s *Server) formatSpotForClient(client *Client, sp *spot.Spot) string {
	if sp == nil {
		return ""
	}
	base := sp.FormatDXCluster()
	if s == nil || s.pathPredictor == nil || !s.pathDisplay {
		return base + "\n"
	}
	glyphs := s.pathGlyphsForClient(client, sp)
	if glyphs != "" {
		base = injectGlyphs(base, glyphs)
	}
	return base + "\n"
}

func (s *Server) pathGlyphsForClient(client *Client, sp *spot.Spot) string {
	if client == nil || sp == nil {
		return ""
	}
	cfg := s.pathPredictor.Config()
	if !cfg.Enabled {
		return ""
	}
	userCell := client.gridCell
	if userCell == pathreliability.InvalidCell && strings.TrimSpace(client.grid) != "" {
		userCell = pathreliability.EncodeCell(client.grid)
		client.gridCell = userCell
	}
	if userCell == pathreliability.InvalidCell {
		return ""
	}
	dxCell := pathreliability.InvalidCell
	if sp.DXCellID != 0 && sp.DXCellID != 0xffff {
		dxCell = pathreliability.CellID(sp.DXCellID)
	}
	if dxCell == pathreliability.InvalidCell {
		dxCell = pathreliability.EncodeCell(sp.DXMetadata.Grid)
	}
	if dxCell == pathreliability.InvalidCell {
		return ""
	}
	userGrid2 := pathreliability.EncodeGrid2(client.grid)
	dxGrid2 := pathreliability.EncodeGrid2(sp.DXMetadata.Grid)

	band := sp.BandNorm
	if strings.TrimSpace(band) == "" {
		band = sp.Band
	}
	mode := sp.ModeNorm
	if strings.TrimSpace(mode) == "" {
		mode = sp.Mode
	}
	now := time.Now().UTC()
	res := s.pathPredictor.Predict(userCell, dxCell, userGrid2, dxGrid2, band, mode, client.noisePenalty, now)
	g := res.Glyph
	return g
}

func injectGlyphs(base string, glyph string) string {
	if len(base) < 66 || len(glyph) < 1 {
		return base
	}
	b := []byte(base)
	// Place the glyph at column 65 (0-based index 64) so there is exactly one
	// space (column 66) before the grid starts at column 67.
	pos := 64
	if pos >= len(b) {
		return base
	}
	// Ensure a single space separates any comment text from the glyph.
	if pos-1 >= 0 {
		b[pos-1] = ' '
	}
	b[pos] = glyph[0]
	return string(b)
}

// applyWelcomeTokens remains for compatibility; it now delegates to the full token replacer.
func applyWelcomeTokens(msg string, now time.Time) string {
	return applyTemplateTokens(msg, templateData{now: now})
}

// applyTemplateTokens replaces supported placeholders in operator-provided templates.
// Tokens:
//
//	<CALL>        -> client callsign
//	<CLUSTER>     -> cluster/node ID
//	<DATE>        -> DD-Mon-YYYY (UTC)
//	<TIME>        -> HH:MM:SS (UTC)
//	<DATETIME>    -> DD-Mon-YYYY HH:MM:SS UTC
//	<UPTIME>      -> uptime since server start (e.g., 3d 04:18:22 or 00:03:05)
//	<USER_COUNT>  -> current connected user count
//	<LAST_LOGIN>  -> previous login timestamp or "(first login)"
//	<LAST_IP>     -> previous login IP or "(unknown)"
func applyTemplateTokens(msg string, data templateData) string {
	if msg == "" {
		return msg
	}
	now := data.now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	date := now.Format("02-Jan-2006")
	tm := now.Format("15:04:05")
	datetime := now.Format("02-Jan-2006 15:04:05 UTC")

	uptime := formatUptime(now, data.startTime)
	if uptime == "" {
		uptime = "unknown"
	}
	userCount := "0"
	if data.userCount > 0 {
		userCount = strconv.Itoa(data.userCount)
	}
	lastLogin := "(first login)"
	if !data.lastLogin.IsZero() {
		lastLogin = data.lastLogin.UTC().Format("02-Jan-2006 15:04:05 UTC")
	}
	lastIP := strings.TrimSpace(data.lastIP)
	if lastIP == "" {
		lastIP = "(unknown)"
	}

	replacer := strings.NewReplacer(
		"<CALL>", data.callsign,
		"<CLUSTER>", data.cluster,
		"<DATETIME>", datetime,
		"<DATE>", date,
		"<TIME>", tm,
		"<UPTIME>", uptime,
		"<USER_COUNT>", userCount,
		"<LAST_LOGIN>", lastLogin,
		"<LAST_IP>", lastIP,
	)
	return replacer.Replace(msg)
}

func formatUptime(now, start time.Time) string {
	if start.IsZero() || now.Before(start) {
		return ""
	}
	dur := now.Sub(start).Round(time.Second)
	days := dur / (24 * time.Hour)
	dur -= days * 24 * time.Hour
	hours := dur / time.Hour
	dur -= hours * time.Hour
	minutes := dur / time.Minute
	dur -= minutes * time.Minute
	seconds := dur / time.Second
	if days > 0 {
		return fmt.Sprintf("%dd %02d:%02d:%02d", days, hours, minutes, seconds)
	}
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
}

func (s *Server) preLoginTemplateData(now time.Time) templateData {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return templateData{
		now:       now,
		startTime: s.startTime,
		userCount: s.GetClientCount(),
	}
}

func (s *Server) postLoginTemplateData(now time.Time, client *Client, prevLogin time.Time, prevIP string) templateData {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	callsign := ""
	if client != nil {
		callsign = client.callsign
	}
	return templateData{
		now:       now,
		startTime: s.startTime,
		userCount: s.GetClientCount(),
		callsign:  callsign,
		cluster:   s.clusterCall,
		lastLogin: prevLogin,
		lastIP:    prevIP,
	}
}

// spotSender sends spots and bulletins to the client from their buffered channels.
func (c *Client) spotSender() {
	spotCh := c.spotChan
	bulletinCh := c.bulletinChan
	for spotCh != nil || bulletinCh != nil {
		select {
		case spot, ok := <-spotCh:
			if !ok {
				spotCh = nil
				continue
			}
			formatted := spot.FormatDXCluster() + "\n"
			if c.server != nil {
				formatted = c.server.formatSpotForClient(c, spot)
			}
			if err := c.Send(formatted); err != nil {
				log.Printf("Error sending spot to %s: %v", c.callsign, err)
				return
			}
		case bulletin, ok := <-bulletinCh:
			if !ok {
				bulletinCh = nil
				continue
			}
			if err := c.Send(bulletin.line); err != nil {
				log.Printf("Error sending bulletin to %s: %v", c.callsign, err)
				return
			}
		}
	}
}

// registerClient adds a client to the active clients list
func (s *Server) registerClient(client *Client) {
	var evicted *Client
	s.clientsMutex.Lock()
	if existing, ok := s.clients[client.callsign]; ok {
		evicted = existing
		delete(s.clients, client.callsign)
	}
	s.clients[client.callsign] = client
	total := len(s.clients)
	s.shardsDirty.Store(true)
	s.clientsMutex.Unlock()

	if evicted != nil {
		msg := strings.TrimSpace(s.duplicateLoginMsg)
		if msg == "" {
			msg = defaultDuplicateLoginMessage
		}
		if !strings.HasSuffix(msg, "\n") {
			msg += "\n"
		}
		_ = evicted.Send(msg)
		evicted.conn.Close()
		log.Printf("Evicted existing session for %s due to duplicate login", client.callsign)
	}
	log.Printf("Registered client: %s (total: %d)", client.callsign, total)
}

// unregisterClient removes a client from the active clients list
func (s *Server) unregisterClient(client *Client) {
	s.clientsMutex.Lock()
	current, ok := s.clients[client.callsign]
	if ok && current == client {
		delete(s.clients, client.callsign)
	}
	total := len(s.clients)
	s.shardsDirty.Store(true)
	s.clientsMutex.Unlock()

	// Ensure all outstanding broadcast deliveries referencing this client complete before we close channels.
	client.pendingDeliveries.Wait()
	client.saveFilter()
	close(client.spotChan)
	close(client.bulletinChan)
	log.Printf("Unregistered client: %s (total: %d)", client.callsign, total)
}

// GetClientCount returns the number of connected clients
func (s *Server) GetClientCount() int {
	s.clientsMutex.RLock()
	defer s.clientsMutex.RUnlock()
	return len(s.clients)
}

// Stop shuts down the telnet server
func (s *Server) Stop() {
	log.Println("Stopping telnet server...")
	close(s.shutdown)
	if s.listener != nil {
		s.listener.Close()
	}

	// Disconnect all clients
	s.clientsMutex.Lock()
	for _, client := range s.clients {
		client.conn.Close()
	}
	s.clientsMutex.Unlock()
}

// SendRaw sends raw bytes to the client
func (c *Client) SendRaw(data []byte) error {
	_, err := c.writer.Write(data)
	if err != nil {
		return err
	}
	return c.writer.Flush()
}

// Send writes a message to the client with proper line endings
func (c *Client) Send(message string) error {
	// Protect broadcast goroutines from stalling on slow or wedged clients by
	// bounding how long a write can block. Each call refreshes the deadline and
	// then clears it, so idle clients are not disconnected by an old timeout.
	if c.conn != nil {
		if err := c.conn.SetWriteDeadline(time.Now().Add(defaultSendDeadline)); err != nil {
			return err
		}
		defer c.conn.SetWriteDeadline(time.Time{})
	}

	// Normalize any existing CRLF to LF, then replace LF with CRLF so callers
	// don't need to worry about line endings (and we avoid doubling CRs).
	message = strings.ReplaceAll(message, "\r\n", "\n")
	message = strings.ReplaceAll(message, "\n", "\r\n")
	_, err := c.writer.WriteString(message)
	if err != nil {
		return err
	}
	return c.writer.Flush()
}

// ReadLine reads a single logical line from the telnet client while enforcing
// three invariants:
//  1. Telnet IAC negotiations are consumed without leaking into user input.
//  2. User-supplied characters are bounded to maxLen bytes to prevent
//     unbounded growth (e.g., 32 bytes for login, 128 for commands).
//  3. Only the whitelisted character set (letters, digits, space, '/', '#',
//     '@', and '-') is accepted. Command contexts can optionally allow commas,
//     wildcards (*), and the '?' confidence glyph so filter commands retain
//     their legacy syntax. Any other
//     character is immediately rejected, logged, and returned as an error so
//     the caller can tear down the session before state is mutated.
//
// The CRLF terminator is always allowed: '\r' is skipped and '\n' ends the
// input. maxLen is measured in bytes because telnet input is ASCII-oriented.
func (c *Client) ReadLine(maxLen int, context string, allowComma, allowWildcard, allowConfidence, allowDot bool) (string, error) {
	if maxLen <= 0 {
		maxLen = defaultCommandLineLimit
	}
	if context == "" {
		context = "command"
	}

	var line []byte

	for {
		b, err := c.reader.ReadByte()
		if err != nil {
			return "", err
		}

		// Handle telnet IAC (Interpret As Command) sequences when using the native backend.
		if c.handleIAC && b == IAC {
			cmd, err := c.reader.ReadByte()
			if err != nil {
				return "", err
			}
			if cmd == DO || cmd == DONT || cmd == WILL || cmd == WONT {
				if _, err := c.reader.ReadByte(); err != nil {
					return "", err
				}
			}
			continue
		}

		// End of line once LF is observed (CR was already skipped).
		if b == '\n' {
			if c.echoInput {
				_, _ = c.writer.WriteString("\r\n")
				_ = c.writer.Flush()
			}
			break
		}
		if b == '\r' {
			continue
		}

		if len(line) >= maxLen {
			c.logRejectedInput(context, fmt.Sprintf("exceeded %d-byte limit", maxLen))
			return "", newInputValidationError(
				fmt.Sprintf("%s input exceeds %d-byte limit", context, maxLen),
				fmt.Sprintf("%s input is too long (maximum %d characters).", friendlyContextLabel(context), maxLen),
			)
		}
		if !isAllowedInputByte(b, allowComma, allowWildcard, allowConfidence, allowDot) {
			c.logRejectedInput(context, fmt.Sprintf("forbidden byte 0x%02X", b))
			return "", newInputValidationError(
				fmt.Sprintf("%s input contains forbidden byte 0x%02X", context, b),
				fmt.Sprintf("%s input may only contain %s.", friendlyContextLabel(context), allowedCharacterList(allowComma, allowWildcard, allowConfidence, allowDot)),
			)
		}

		if c.echoInput {
			if err := c.writer.WriteByte(b); err != nil {
				return "", err
			}
			if err := c.writer.Flush(); err != nil {
				return "", err
			}
		}
		line = append(line, b)
	}

	return string(line), nil
}

// isAllowedInputByte reports whether the byte is part of the strict ingress
// safe list (letters, digits, space, '/', '#', '@', '-'). When allowComma is
// true, comma is also accepted to preserve legacy comma-delimited filter syntax.
// allowDot permits '.' for numeric inputs and spot comments. CRLF is handled
// separately by ReadLine.
func newInputValidationError(reason, userMessage string) error {
	return &InputValidationError{
		reason:      reason,
		userMessage: userMessage,
	}
}

func isAllowedInputByte(b byte, allowComma, allowWildcard, allowConfidence, allowDot bool) bool {
	switch {
	case b >= 'A' && b <= 'Z':
		return true
	case b >= 'a' && b <= 'z':
		return true
	case b >= '0' && b <= '9':
		return true
	case b == ' ':
		return true
	case b == '/':
		return true
	case b == '#':
		return true
	case b == '@':
		return true
	case b == '-':
		return true
	case allowDot && b == '.':
		return true
	case allowComma && b == ',':
		return true
	case allowWildcard && b == '*':
		return true
	case allowConfidence && b == '?':
		return true
	default:
		return false
	}
}

func allowedCharacterList(allowComma, allowWildcard, allowConfidence, allowDot bool) string {
	base := "letters, numbers, space, '/', '#', '@', '-'"
	if allowComma {
		base += ", ','"
	}
	if allowWildcard {
		base += ", '*'"
	}
	if allowConfidence {
		base += ", '?'"
	}
	if allowDot {
		base += ", '.'"
	}
	return base
}

func spotterIP(address string) string {
	if address == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return strings.TrimSpace(address)
	}
	return host
}

func friendlyContextLabel(context string) string {
	context = strings.TrimSpace(context)
	if context == "" {
		return "Input"
	}
	if len(context) == 1 {
		return strings.ToUpper(context)
	}
	return strings.ToUpper(context[:1]) + context[1:]
}

// logRejectedInput emits a consistent, high-signal log entry whenever the
// ingress guardrail rejects input. This makes debugging user issues easier and
// provides an audit trail when a hostile client repeatedly violates the
// policy. The helper deliberately prefers the callsign when known, falling
// back to the remote address prior to login.
func (c *Client) logRejectedInput(context, reason string) {
	id := strings.TrimSpace(c.callsign)
	if id == "" {
		id = c.address
	}
	log.Printf("Rejected %s input from %s: %s", context, id, reason)
}
