Blind reference decompositions for gin, httpie, storefront, express, click, hyperfine (from source only; no .abbd, CLAUDE.md, or git consulted).

## gin
path	top	inner
gin.go	engine	engine core
mode.go	engine	mode & debug
debug.go	engine	mode & debug
utils.go	engine	handler adapters & helpers
version.go	engine	package info
doc.go	engine	package info
routergroup.go	router	route groups
tree.go	router	radix tree
path.go	router	path cleaning
fs.go	router	static file serving
context.go	context	request context
context_appengine.go	context	request context
deprecated.go	context	request context
errors.go	context	error collection
response_writer.go	context	response writer
auth.go	middleware	-
logger.go	middleware	-
recovery.go	middleware	-
binding/	binding	body decoders
binding/binding.go	binding	binding interface & registry
binding/binding_nomsgpack.go	binding	binding interface & registry
binding/default_validator.go	binding	validation
binding/form.go	binding	form & URL mapping
binding/form_mapping.go	binding	form & URL mapping
binding/multipart_form_mapping.go	binding	form & URL mapping
binding/query.go	binding	form & URL mapping
binding/header.go	binding	form & URL mapping
binding/uri.go	binding	form & URL mapping
render/	render	structured data renderers
render/render.go	render	render interface
render/html.go	render	HTML templates
render/text.go	render	text & raw data
render/data.go	render	text & raw data
render/reader.go	render	text & raw data
render/pdf.go	render	text & raw data
render/redirect.go	render	redirect
codec/json/	json codec	pluggable backends
codec/json/api.go	json codec	codec API
ginS/	ginS	-
internal/	internal	-
(binding/ default row covers json, xml, yaml, toml, msgpack, protobuf, bson, plain; render/ default row covers json, xml, yaml, toml, msgpack, protobuf, bson. testdata/ is excluded as test data.)

Notes: Least sure about splitting the root package into engine, router, context, and middleware. I followed the file roles: the radix tree and route groups are the routing concern, and Context plus ResponseWriter plus errors are the per-request concern. fs.go could equally go to engine. ginS and internal are tiny, and a hand-drawn diagram might fold them into engine as side notes.

## httpie
path	top	inner
httpie/__init__.py	core	entry
httpie/__main__.py	core	entry
httpie/core.py	core	program main
httpie/context.py	core	environment & config
httpie/config.py	core	environment & config
httpie/status.py	core	exit status
httpie/utils.py	core	shared utils
httpie/compat.py	core	shared utils
httpie/cli/	cli	argument parser
httpie/cli/definition.py	cli	option definitions
httpie/cli/options.py	cli	option definitions
httpie/cli/constants.py	cli	option definitions
httpie/cli/requestitems.py	cli	request items
httpie/cli/dicts.py	cli	request items
httpie/cli/nested_json/	cli	nested JSON
httpie/client.py	client	request building
httpie/adapters.py	client	transport & SSL
httpie/ssl_.py	client	transport & SSL
httpie/uploads.py	client	uploads
httpie/models.py	client	message models & encoding
httpie/encoding.py	client	message models & encoding
httpie/sessions.py	sessions	session store
httpie/cookies.py	sessions	cookie policy
httpie/legacy/	sessions	legacy format migration
httpie/downloads.py	downloads	-
httpie/output/	output	writer & streams
httpie/output/processing.py	output	formatters
httpie/output/formatters/	output	formatters
httpie/output/utils.py	output	formatters
httpie/output/lexers/	output	lexers
httpie/output/ui/	output	terminal UI
httpie/plugins/	plugins	plugin manager
httpie/plugins/base.py	plugins	plugin base classes
httpie/plugins/builtin.py	plugins	builtin auth plugins
httpie/manager/	httpie manager	entry & dispatch
httpie/manager/cli.py	httpie manager	CLI spec
httpie/manager/tasks/	httpie manager	other tasks
httpie/manager/tasks/plugins.py	httpie manager	plugin installer
httpie/manager/tasks/sessions.py	httpie manager	session upgrade
httpie/internal/	update checker	daemon spawning
httpie/internal/update_warnings.py	update checker	update warnings
httpie/internal/__build_channel__.py	update checker	update warnings
(Excluded: setup.py, extras/ packaging, profiling, and man-page scripts.)

Notes: config.py could be its own top part, since the config directory is a documented concept. I folded it into core next to Environment, which is what loads it. models.py and encoding.py are shared by client and output, and I put them with client because they wrap the requests objects. I folded downloads into neither part; it stays separate because --download is a distinct documented mode.

## storefront
path	top	inner
app/	app routes	root layout & home
app/product/	app routes	product page
app/search/	app routes	search & collections
app/[page]/	app routes	CMS pages
app/robots.ts	app routes	SEO & API routes
app/sitemap.ts	app routes	SEO & API routes
app/api/	app routes	SEO & API routes
lib/shopify/	shopify	types
lib/shopify/index.ts	shopify	client & operations
lib/shopify/queries/	shopify	queries
lib/shopify/mutations/	shopify	mutations
lib/shopify/fragments/	shopify	fragments
lib/	shared lib	-
components/cart/	cart	-
components/product/	product	-
components/layout/	layout	product grid items
components/layout/navbar/	layout	navbar
components/layout/footer.tsx	layout	footer
components/layout/footer-menu.tsx	layout	footer
components/layout/search/	layout	search & filters
components/grid/	merchandising	-
components/carousel.tsx	merchandising	-
components/	ui primitives	-
(Excluded: next.config.ts and postcss.config.mjs, which are build config. The components/ default row covers label, price, prose, loading-dots, logo-square, icons/, opengraph-image, and welcome-toast.)

Notes: cart has 7 files but I left it without inner parts. A split into server actions (actions.ts), state (cart-context.tsx), and UI (the rest) is defensible. Merchandising (the grid plus the carousel) is my own name for the home-page product displays, and could instead fold into layout or ui primitives. shared lib could fold into shopify, because constants.ts is mostly Shopify config.

## express
path	top	inner
index.js	factory	-
lib/express.js	factory	-
lib/application.js	application	-
lib/request.js	request	-
lib/response.js	response	-
lib/view.js	view	-
lib/utils.js	utils	-

Notes: Express 5 delegates routing to the external `router` package, so there is no router part in this tree, though a hand-drawn diagram would show it as an external dependency of application. response.js is one large file with separate concerns (sending and serialization, headers and cookies, files and downloads, redirects, rendering). A file-level table can't show that split. utils could be dropped as a part and noted as helpers.

## click
path	top	inner
src/click/__init__.py	core	-
src/click/core.py	core	-
src/click/globals.py	core	-
src/click/decorators.py	decorators	-
src/click/types.py	parameter types	-
src/click/parser.py	parser	-
src/click/exceptions.py	exceptions	-
src/click/formatting.py	help formatting	-
src/click/_textwrap.py	help formatting	-
src/click/termui.py	terminal UI	-
src/click/_termui_impl.py	terminal UI	-
src/click/shell_completion.py	shell completion	-
src/click/testing.py	testing	-
src/click/utils.py	utilities	public utils
src/click/_compat.py	utilities	stream compat
src/click/_winconsole.py	utilities	windows console
src/click/_utils.py	utilities	sentinels

Notes: core.py (about 4000 lines) is one file with clearly separate concerns: Context, Command, Group, and Parameter with Option and Argument. Its inner parts can only be named at symbol level, not at file level. testing.py ships as the public CliRunner, so I counted it as product. Ten top parts is at the upper limit, and exceptions could fold into core.

## hyperfine
path	top	inner
src/main.rs	cli	entry
src/cli.rs	cli	argument definition
src/options.rs	cli	options
src/error.rs	cli	errors
src/command.rs	commands	command list
src/parameter/	commands	parameter scans
src/shell_syntax.rs	commands	shell syntax checks
src/benchmark/mod.rs	benchmark	benchmark run
src/benchmark/scheduler.rs	benchmark	scheduler
src/benchmark/executor.rs	benchmark	executors
src/benchmark/measurement.rs	benchmark	measurements & results
src/benchmark/benchmark_result.rs	benchmark	measurements & results
src/timer/mod.rs	timer	timer interface
src/timer/wall_clock_timer.rs	timer	wall clock
src/timer/unix_timer.rs	timer	unix CPU timer
src/timer/linux_counters.rs	timer	unix CPU timer
src/timer/macos_counters.rs	timer	unix CPU timer
src/timer/windows_timer.rs	timer	windows CPU timer
src/metric.rs	statistics	metrics
src/quantity/mod.rs	statistics	quantities & units
src/quantity/statistics.rs	statistics	descriptive stats
src/outlier_detection.rs	statistics	outlier detection
src/export/	export	markup formats
src/export/mod.rs	export	export manager
src/export/csv.rs	export	csv
src/export/json.rs	export	json
src/export/json/	export	json
src/output/	output	-
src/util/	util	-
(The export/ default row covers markup.rs, markdown, asciidoc, and orgmode. Excluded: src/export/tests.rs, build.rs for completions, and scripts/ for the Python plotting and analysis companions.)

Notes: metric.rs could sit with benchmark rather than statistics, but its own doc comment says "metric selection, descriptive statistics, display units". shell_syntax.rs is used by main for validation, and I grouped it with commands because it inspects command lines. util is small and an engineer might not name it; scripts/ could arguably count as a product companion, because the README documents them.