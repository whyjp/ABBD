#ifndef CORE_H
#define CORE_H

struct state {
	int a;
};

struct source_info {
	const char *name;
	int (*create)(int n);
	void (*destroy)(void);
};

void register_source(const struct source_info *info);
int create_all(int n);
int core_run(struct state *s);

#endif
