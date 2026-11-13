#include <stddef.h>
#include "core.h"

static const struct source_info *sources[4];
static int count;

void register_source(const struct source_info *info) { sources[count++] = info; }

int create_all(int n) {
	int total = 0;
	for (int i = 0; i < count; i++)
		total += sources[i]->create(n);
	return total;
}
