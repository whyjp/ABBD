from shapes.shape import Shape


class Circle(Shape):
    def __init__(self, r):
        self.r = r

    def area(self):
        return self.r * self.r * 3
