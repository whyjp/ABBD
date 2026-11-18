import { add, LIMIT } from "../util/util";
import { Circle } from "../circle/circle";
import type { Shape } from "../shapes/shape";
import { twice } from "../lib/helper.js";

const s: Shape = new Circle(2);
export const total: number = add(LIMIT, s.area()) + twice(1);
