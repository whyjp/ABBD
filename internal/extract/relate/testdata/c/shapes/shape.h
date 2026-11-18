#ifndef SHAPE_H
#define SHAPE_H

class Shape {
public:
	virtual ~Shape() {}
	virtual int area() const = 0;
};

#endif
