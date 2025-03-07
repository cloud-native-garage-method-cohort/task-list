package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

var (
	invalidParamsDeadline = errors.New("could not execute deadline. Usage: deadline <taskId> <dateAsString>")
)

/*
 * Features to add
 *
 * 1. Deadlines
 *    (i)   Give each task an optional deadline with the 'deadline <ID> <date>' command.
 *    (ii)  Show all tasks due today with the 'today' command.
 * 2. Customisable IDs
 *    (i)   Allow the user to specify an identifier that's not a number.
 *    (ii)  Disallow spaces and special characters from the ID.
 * 3. Deletion
 *    (i)   Allow users to delete tasks with the 'delete <ID>' command.
 * 4. Views
 *    (i)   View tasks by date with the 'view by date' command.
 *    (ii)  View tasks by deadline with the 'view by deadline' command.
 *    (iii) Don't remove the functionality that allows users to view tasks by project,
 *          but change the command to 'view by project'
 */

type Error string

func (e Error) Error() string {
	return string(e)
}

const (
	// Quit is the text command used to quit the task manager.
	TaskNotFoundErr        = Error("Task not found")
	Quit            string = "quit"
	prompt          string = "> "
)

// TaskList is a set of tasks, grouped by project.
type TaskList struct {
	in  io.Reader
	out io.Writer

	projectTasks map[string][]*Task
	lastID       int64
}

// NewTaskList initializes a TaskList on the given I/O descriptors.
func NewTaskList(in io.Reader, out io.Writer) *TaskList {
	return &TaskList{
		in:           in,
		out:          out,
		projectTasks: make(map[string][]*Task),
		lastID:       0,
	}
}

// Run runs the command loop of the task manager.
// Sequentially executes any given command, until the user types the Quit message.
func (l *TaskList) Run(errorsChan chan<- error, shutdownChan chan bool) {
	scanner := bufio.NewScanner(l.in)

	fmt.Fprint(l.out, prompt)
	for scanner.Scan() {
		cmdLine := scanner.Text()
		if cmdLine == Quit {
			shutdownChan <- true
			return
		}

		err := l.execute(cmdLine)
		if err != nil {
			errorsChan <- err
			fmt.Printf("program exited, %v", err)
		}
		fmt.Fprint(l.out, prompt)
	}
}

func (l *TaskList) execute(cmdLine string) error {
	args := strings.Split(cmdLine, " ")
	command := args[0]
	switch command {
	case "show":
		l.show()
	case "add":
		if len(args) < 2 {
			return fmt.Errorf("could not execute add, it requires at least 2 parameters")
		}
		l.add(args[1:])
	case "check":
		l.check(args[1])
	case "uncheck":
		l.uncheck(args[1])
	case "help":
		l.help()
	case "deadline":
		if len(args) < 2 {
			return fmt.Errorf("could not execute deadline. Usage: deadline <taskId> <dateAsString>")
		}
		l.deadline(args[1], args[2])
	case "today":
		l.today()
	default:
		l.error(command)
	}
	return nil
}

func (l *TaskList) help() {
	fmt.Fprintln(l.out, `Commands:
  show
  add project <project name>
  add task <project name> <task description>
  check <task ID>
  uncheck <task ID>
  `)
}

func (l *TaskList) error(command string) {
	fmt.Fprintf(l.out, "Unknown command \"%s\".\n", command)
}

func (l *TaskList) today() {
	// sort projects (to make output deterministic)
	sortedProjects := make([]string, 0, len(l.projectTasks))
	for project := range l.projectTasks {
		sortedProjects = append(sortedProjects, project)
	}
	sort.Sort(sort.StringSlice(sortedProjects))

	// show projects sequentially
	for _, project := range sortedProjects {
		tasks := l.projectTasks[project]
		fmt.Fprintf(l.out, "%s\n", project)
		for _, task := range tasks {
			if task.IsPreviousToCurrentDate() {
				done := ' '
				if task.IsDone() {
					done = 'X'
				}
				fmt.Fprintf(l.out, "    [%c] %d:%s %s\n", done, task.GetID(), task.GetDeadline(), task.GetDescription())
			}
		}
		fmt.Fprintln(l.out)
	}
}

func (l *TaskList) show() {
	// sort projects (to make output deterministic)
	sortedProjects := make([]string, 0, len(l.projectTasks))
	for project := range l.projectTasks {
		sortedProjects = append(sortedProjects, project)
	}
	sort.Sort(sort.StringSlice(sortedProjects))

	// show projects sequentially
	for _, project := range sortedProjects {
		tasks := l.projectTasks[project]
		fmt.Fprintf(l.out, "%s\n", project)
		for _, task := range tasks {
			done := ' '
			if task.IsDone() {
				done = 'X'
			}
			fmt.Fprintf(l.out, "    [%c] %d:%s %s\n", done, task.GetID(), task.GetDeadline(), task.GetDescription())
		}
		fmt.Fprintln(l.out)
	}
}

func (l *TaskList) add(args []string) {
	projectName := args[1]
	if args[0] == "project" {
		l.addProject(projectName)
	} else if args[0] == "task" {
		description := strings.Join(args[2:], " ")
		l.addTask(projectName, description)
	}
}

func (l *TaskList) addProject(name string) {
	l.projectTasks[name] = make([]*Task, 0)
}

func (l *TaskList) addTask(projectName, description string) {
	tasks, ok := l.projectTasks[projectName]
	if !ok {
		fmt.Fprintf(l.out, "Could not find a project with the name \"%s\".\n", projectName)
		return
	}
	l.projectTasks[projectName] = append(tasks, NewTask(l.nextID(), description, false))
}

func (l *TaskList) check(idString string) {
	l.setDone(idString, true)
}

func (l *TaskList) uncheck(idString string) {
	l.setDone(idString, false)
}

func (l *TaskList) setDone(idString string, done bool) {
	task, err := l.getTaskBy(idString)
	if err != nil {
		return
	}
	task.done = done
}

func (l *TaskList) getTaskBy(idString string) (*Task, error) {
	id, err := NewIdentifier(idString)
	if err != nil {
		fmt.Fprintf(l.out, "Invalid ID \"%s\".\n", idString)
		return nil, err
	}

	for _, tasks := range l.projectTasks {
		for _, task := range tasks {
			if task.GetID() == id {
				return task, nil
			}
		}
	}

	fmt.Fprintf(l.out, "Task with ID \"%d\" not found.\n", id)
	return nil, TaskNotFoundErr
}

func (l *TaskList) nextID() int64 {
	l.lastID++
	return l.lastID
}

func (l *TaskList) deadline(id string, deadlineString string) {
	deadline, err := NewDeadline(deadlineString)
	if err != nil {
		return
	}

	task, err := l.getTaskBy(id)
	if err != nil {
		return
	}

	task.deadline = deadline
}
