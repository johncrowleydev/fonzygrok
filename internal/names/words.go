// Package names provides human-friendly name generation and validation
// for tunnel subdomains. Names follow the "adjective-noun" pattern
// (e.g., "calm-tiger", "swift-falcon") and are selected using crypto/rand.
package names

// adjectives is a curated list of short, friendly adjectives for name generation.
// All entries are lowercase, 3-7 characters, family-friendly, and easy to type.
var adjectives = []string{
	"able", "agile", "avid", "bold", "brave",
	"brief", "broad", "brisk", "calm", "civic",
	"clean", "clear", "close", "cool", "crisp",
	"curly", "deft", "dense", "eager", "easy",
	"epic", "even", "fair", "fancy", "fast",
	"fine", "firm", "flat", "fleet", "fluffy",
	"fond", "frank", "fresh", "glad", "grand",
	"great", "green", "handy", "happy", "hardy",
	"high", "ideal", "jolly", "keen", "kind",
	"large", "lean", "light", "lively", "lucky",
	"main", "merry", "mild", "modern", "neat",
	"new", "nice", "noble", "novel", "open",
	"plain", "plush", "polite", "prime", "proud",
	"pure", "quick", "quiet", "rapid", "ready",
	"rich", "royal", "sharp", "shiny", "smart",
	"solid", "sunny", "super", "sweet", "swift",
	"tidy", "tough", "vivid", "warm", "wise",
}

// nouns is a curated list of short, friendly nouns for name generation.
// All entries are lowercase, 3-7 characters, family-friendly, and easy to type.
var nouns = []string{
	"acorn", "aspen", "badge", "basil", "berry",
	"birch", "blaze", "bloom", "bolt", "breeze",
	"brook", "canoe", "cedar", "chime", "cliff",
	"cloud", "cobra", "comet", "coral", "crane",
	"creek", "crest", "daisy", "dune", "eagle",
	"ember", "fern", "finch", "flame", "flint",
	"forge", "frost", "glade", "grove", "hawk",
	"haven", "heron", "hill", "iris", "isle",
	"jade", "jewel", "lark", "leaf", "lily",
	"lotus", "maple", "marsh", "mesa", "mist",
	"moose", "nectar", "oak", "opal", "otter",
	"panda", "peak", "pearl", "pine", "plume",
	"pond", "quail", "raven", "reef", "ridge",
	"river", "robin", "rover", "sage", "shore",
	"slate", "snowy", "spark", "spire", "stone",
	"stork", "storm", "swift", "thorn", "tiger",
	"trail", "tulip", "vale", "wave", "wren",
}
