#ifndef CIRCLE_H
#define CIRCLE_H

#include "shape.h"

class Circle : public Shape {
public:
	explicit Circle(int r) : r_(r) {}
	int area() const override;

private:
	int r_;
};

Shape *make_circle(int r);

#endif
