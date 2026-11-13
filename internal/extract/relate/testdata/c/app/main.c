#include "api.h"
#include "core.h"
#include "util.h"

void plugin_init(void);
int extra_go(void);

static const util_order orders[] = {util_desc};

int main(void) {
	struct state s = {1};
	plugin_init();
#ifdef APP_EXTRA
	extra_go();
#endif
	return core_run(&s) + util_add(1, 2) + API_TWICE(orders[0](1, 2));
}
