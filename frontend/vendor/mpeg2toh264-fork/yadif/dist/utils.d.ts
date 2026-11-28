/** One triangle over the whole viewport: every pixel is the fragment shader's. */
export declare const VERTEX_SHADER = "#version 300 es\nvoid main() {\n  // From the vertex index alone. There is no geometry here worth a buffer.\n  vec2 corner = vec2((gl_VertexID << 1) & 2, gl_VertexID & 2);\n  gl_Position = vec4(corner * 2.0 - 1.0, 0.0, 1.0);\n}\n";
export declare function createProgram(gl: WebGL2RenderingContext, fragmentSource: string, vertexSource: string): WebGLProgram;
/**
 * Draw with `program` once, into a 1×1 framebuffer of `outputs` textures of
 * `format`: as many as the program writes, of the format it writes in use.
 *
 * Linking is not the end of compiling. A driver may compile a program for the
 * GPU only when it is first drawn with, for the framebuffer it is drawn into:
 * Firefox on Windows (ANGLE over Direct3D 11) takes 50-65 ms there for the
 * film detector's larger programs. Done at their first picture, that holds up
 * the frame the picture is due in, and the pictures due meanwhile are passed
 * over; done here, as the program is made, it is part of setting up.
 *
 * Right after linking every sampler reads texture unit 0, which gets a 1×1
 * texture, so that the draw reads nothing incomplete.
 */
export declare function drawOnce(gl: WebGL2RenderingContext, program: WebGLProgram, format: GLenum, outputs?: number): void;
export declare function compile(gl: WebGL2RenderingContext, kind: GLenum, source: string): WebGLShader;
