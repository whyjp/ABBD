import { Base, Shape } from "../shapes/shape";

export class Circle extends Base implements Shape {
  constructor(private r: number) {
    super();
  }
  area(): number {
    return this.r * this.r * 3;
  }
  name(): string {
    return "circle";
  }
}
