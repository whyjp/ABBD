#include "core.h"

struct local {
	int n;
};

int core_run(struct state *s) {
	struct local l = {s->a};
	return create_all(l.n);
}
