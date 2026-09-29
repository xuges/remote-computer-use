/* A deterministic remote GUI fixture. X11 is a test dependency only. */
#include <X11/Xlib.h>
#include <X11/keysym.h>
#include <stdio.h>
#include <stdlib.h>
int main(void){
    Display*d=XOpenDisplay(NULL);if(!d)return 1;int screen=DefaultScreen(d);
    Window w=XCreateSimpleWindow(d,RootWindow(d,screen),0,0,640,480,0,0,0);
    XStoreName(d,w,"RCU integration fixture");
    XSelectInput(d,w,ExposureMask|KeyPressMask|KeyReleaseMask|ButtonPressMask|ButtonReleaseMask|PointerMotionMask);
    XMapWindow(d,w);XSetInputFocus(d,w,RevertToParent,CurrentTime);GC gc=XCreateGC(d,w,0,NULL);XFlush(d);
    setvbuf(stdout,NULL,_IOLBF,0);
    for(;;){XEvent e;XNextEvent(d,&e);
        if(e.type==Expose){XSetForeground(d,gc,0xff0000);XFillRectangle(d,w,gc,0,0,320,240);XSetForeground(d,gc,0x00ff00);XFillRectangle(d,w,gc,320,0,320,240);XSetForeground(d,gc,0x0000ff);XFillRectangle(d,w,gc,0,240,320,240);XSetForeground(d,gc,0xffffff);XFillRectangle(d,w,gc,320,240,320,240);XFlush(d);}
        if(e.type==MotionNotify)printf("move %d %d\n",e.xmotion.x,e.xmotion.y);
        if(e.type==ButtonPress||e.type==ButtonRelease)printf("button %s %u %d %d\n",e.type==ButtonPress?"down":"up",e.xbutton.button,e.xbutton.x,e.xbutton.y);
        if(e.type==KeyPress||e.type==KeyRelease){KeySym sym=XLookupKeysym(&e.xkey,0);printf("key %s %lu\n",e.type==KeyPress?"down":"up",sym);if(e.type==KeyPress){XSetForeground(d,gc,0xffff00);XFillRectangle(d,w,gc,10,10,20,20);XFlush(d);}}
    }
}
