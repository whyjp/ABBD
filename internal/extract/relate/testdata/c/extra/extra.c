struct state {
	long b;
};

int extra_go(void) {
	struct state s = {7};
	return (int)s.b;
}

int dup_name(void) { return 1; }
