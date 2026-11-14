struct state {
	long b;
};

int extra_go(void) {
	struct state s = {7};
	return (int)s.b;
}
