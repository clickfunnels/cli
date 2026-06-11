package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/clickfunnels/clickfunnels-cli/internal/api"
	"github.com/clickfunnels/clickfunnels-cli/internal/output"
	"github.com/clickfunnels/clickfunnels-cli/internal/ui"
)

// Blogs::Post is a child resource nested under a Blog: the API route is
// POST /blogs/{blog_id}/posts, so the parent (blog) is a *path* parameter, not
// a body field. In the generated client that surfaces as the first method
// argument (blogId int); in the CLI it's the required --blog flag. The request
// body carries only the post's own fields.
//
// NOTE: the generated method is CreateBlogPostWithResponse — the spec's
// operationId (createBlogPost) is singular "Blog", inconsistent with the
// Blogs::Post controller/model convention and with its own sibling
// listBlogsPosts. That's a spec bug (see below); the method name inherits it.

func newBlogsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "blogs",
		Short: "Manage blogs",
	}
	cmd.AddCommand(newBlogsPostsCmd())
	return cmd
}

func newBlogsPostsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "posts",
		Short: "Manage blog posts (nested under a blog)",
	}
	cmd.AddCommand(newBlogsPostsListCmd())
	cmd.AddCommand(newBlogsPostsCreateCmd())
	return cmd
}

func blogPostColumns() []output.Column[api.BlogsPostAttributes] {
	return []output.Column[api.BlogsPostAttributes]{
		{Header: "ID", Value: func(p api.BlogsPostAttributes) string { return p.PublicId }},
		{Header: "TITLE", Value: func(p api.BlogsPostAttributes) string { return p.Title }},
		{Header: "VISIBILITY", Value: func(p api.BlogsPostAttributes) string { return string(p.Visibility) }},
	}
}

func newBlogsPostsListCmd() *cobra.Command {
	var (
		blog  int
		after string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List posts in a blog",
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := outputFormat(cmd)
			if err != nil {
				return err
			}
			client, _, err := authedClient(cmd)
			if err != nil {
				return err
			}

			params := &api.ListBlogsPostsParams{}
			if after != "" {
				a := api.After(after)
				params.After = &a
			}
			// blog (the parent) is passed as the path arg, not in any body.
			resp, err := client.ListBlogsPostsWithResponse(cmd.Context(), blog, params)
			if err != nil {
				return err
			}
			posts := derefSlice(resp.JSON200)

			if format == output.Table && len(posts) == 0 {
				fmt.Println(ui.Subtle.Render("No posts found."))
				return nil
			}
			if err := output.Collection(os.Stdout, format, blogPostColumns(), posts); err != nil {
				return err
			}
			if format == output.Table {
				if next := api.Cursor(resp.HTTPResponse); next != "" {
					fmt.Println(ui.Subtle.Render(fmt.Sprintf("\nMore results — fetch with: cf blogs posts list --blog %d --after %s", blog, next)))
				}
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&blog, "blog", 0, "parent blog id (required)")
	cmd.Flags().StringVar(&after, "after", "", "pagination cursor from a previous page")
	_ = cmd.MarkFlagRequired("blog")
	return cmd
}

func newBlogsPostsCreateCmd() *cobra.Command {
	var (
		blog               int
		title, markup, vis string
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a post in a blog",
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := outputFormat(cmd)
			if err != nil {
				return err
			}
			client, _, err := authedClient(cmd)
			if err != nil {
				return err
			}

			// Only the child's own fields go in the body; the parent blog is the
			// path arg below.
			post := api.BlogsPostParameters{
				Title:  nonEmpty(title),
				Markup: nonEmpty(markup),
			}
			if vis != "" {
				v := api.BlogsPostParametersVisibility(vis)
				post.Visibility = &v
			}
			body := api.CreateBlogPostJSONRequestBody{BlogsPost: &post}

			resp, err := client.CreateBlogPostWithResponse(cmd.Context(), blog, &api.CreateBlogPostParams{}, body)
			if err != nil {
				return err
			}
			if format != output.Table {
				return output.Object(os.Stdout, format, resp.JSON201)
			}
			fmt.Printf("%s Created post %s %s\n",
				ui.Success.Render(ui.Check),
				ui.Accent.Render(resp.JSON201.PublicId),
				ui.Subtle.Render(resp.JSON201.Title))
			return nil
		},
	}
	cmd.Flags().IntVar(&blog, "blog", 0, "parent blog id (required)")
	cmd.Flags().StringVar(&title, "title", "", "post title")
	cmd.Flags().StringVar(&markup, "markup", "", "PML markup for the post content")
	cmd.Flags().StringVar(&vis, "visibility", "", "draft | public | members_only | scheduled")
	_ = cmd.MarkFlagRequired("blog")
	return cmd
}
