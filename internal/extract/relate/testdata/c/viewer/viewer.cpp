#include "circle.h"

int view(int r) {
	Shape *s = make_circle(r);
	int a = s->area();
	delete s;
	return a;
}
