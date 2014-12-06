#include "_cgo_export.h"

void error_panic(j_common_ptr cinfo) {
  struct { const char *p; } a;
  char buffer[JMSG_LENGTH_MAX];
  (*cinfo->err->format_message) (cinfo, buffer);
  goPanic(buffer);
}

typedef struct {
  unsigned char *buf;
  unsigned long buf_size;
} mem_helper;

mem_helper *alloc_mem_helper() {
  return (mem_helper*) calloc(1,sizeof(mem_helper));
}
