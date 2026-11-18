import fs from "node:fs";
import path from "path";
import { pad } from "../lib/lib";
import { left } from "leftpad";

export const out: string = pad(path.join("a", fs.existsSync("b") ? "c" : left("d")));
