#include "circle.h"

int Circle::area() const { return 3 * r_ * r_; }

Shape *make_circle(int r) { return new Circle(r); }
