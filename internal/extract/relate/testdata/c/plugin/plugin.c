#include "api.h"
#include "core.h"

struct local {
	int n;
};

static int plug_create(int n) {
	struct local l = {n};
	return util_add(l.n, 1);
}

static void plug_destroy(void) {}

static const struct source_info plugin_source = {
	"plug",
	plug_create,
	plug_destroy,
};

void plugin_init(void) {
#if API_VERSION > 2
	register_source(&plugin_source);
#endif
}
