from circle.circle import Circle
from priv.priv import _secret
from shapes import shape
from util.maths import LIMIT, add

s: shape.Shape = Circle(2)
total = add(LIMIT, s.area()) + _secret()
