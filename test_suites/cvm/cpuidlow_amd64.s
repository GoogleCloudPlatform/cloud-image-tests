//go:build amd64
#include "textflag.h"

// func cpuidlow(leaf, subleaf uint32) (eax, ebx, ecx, edx uint32)
TEXT ·cpuidlow(SB), NOSPLIT, $0-24
	MOVL leaf+0(FP), AX
	MOVL subleaf+4(FP), CX
	CPUID
	MOVL AX, eax+8(FP)
	MOVL BX, ebx+12(FP)
	MOVL CX, ecx+16(FP)
	MOVL DX, edx+20(FP)
	RET
