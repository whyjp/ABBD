#include <stdio.h>
#include <string.h>

#include "lib.h"

int main(void) {
	char a[4], b[4] = "abc";
	memcpy(a, b, 4);
	printf("%s %d\n", a, lib_size());
	return (int)strlen(a);
}
